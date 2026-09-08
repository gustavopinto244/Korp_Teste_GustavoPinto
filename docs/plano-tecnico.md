# Plano técnico — Sistema de Emissão de Notas Fiscais

Repositório de entrega: [`gustavopinto244/Korp_Teste_GustavoPinto`](https://github.com/gustavopinto244/Korp_Teste_GustavoPinto).

Este documento é a referência técnica do desafio. Foi escrito antes da primeira
linha de código e **mantido em sincronia com o que foi implementado**: onde a
implementação divergiu do plano, o texto descreve o que existe hoje e registra
a divergência, em vez de preservar a intenção original. Ele detalha, em nível
de implementação, as decisões já fixadas no `CLAUDE.md` da raiz (Go nos dois
microsserviços, PostgreSQL, schemas separados por serviço, estoque confirma a
baixa antes de a nota fechar, chave de idempotência determinística por nota,
Angular Material, Docker Compose com falha demonstrável ao vivo, IA por
linguagem natural na criação de nota).

Para *como rodar*, ver [`README.md`](../README.md); para as respostas ao
checklist do enunciado, [`DETALHAMENTO_TECNICO.md`](../DETALHAMENTO_TECNICO.md).

---

## 1. Modelo de dados completo

### 1.0 Como o schema é resolvido (vale para 1.1 e 1.2)

Nenhuma migration nomeia o schema em que escreve. A SQL é **não qualificada**
e o schema alvo vem do `search_path` da conexão (`DATABASE_URL` em produção,
`TEST_DATABASE_URL_*` nos testes). Quem resolve isso é o runner próprio
`internal/migrate`, presente nos dois serviços:

1. lê `current_setting('search_path')` e toma o primeiro schema utilizável —
   `estoque` em produção, `estoque_test` na suíte;
2. executa `CREATE SCHEMA IF NOT EXISTS` nesse nome (por isso não existe
   `CREATE SCHEMA` dentro de nenhum `.sql`: o nome é dinâmico e não caberia num
   arquivo estático);
3. mantém a tabela de controle `schema_migrations` **dentro do schema do
   serviço**, não em `public` — uma tabela de controle compartilhada entre os
   dois microsserviços impediria qualquer isolamento real;
4. aplica cada migration pendente em uma transação, precedida de
   `SET LOCAL search_path`, e registra a versão aplicada na mesma transação.

O ganho concreto: o mesmo arquivo `.sql` serve ao schema de produção e a um
schema de teste descartável, então a suíte de integração pode derrubar e
recriar o schema inteiro a cada execução sem tocar nos dados da demo. Os blocos
abaixo mostram os objetos como as migrations os criam (sem prefixo), com o
schema de produção indicado no título de cada seção.

### 1.1 Serviço `estoque` — schema `estoque`

```sql
-- migrations/0001_create_schema_e_produto.sql
CREATE TABLE produto (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    codigo        VARCHAR(50)  NOT NULL,
    descricao     VARCHAR(200) NOT NULL,
    saldo         INTEGER      NOT NULL DEFAULT 0,
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    atualizado_em TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_produto_codigo UNIQUE (codigo),
    CONSTRAINT ck_produto_saldo_nao_negativo CHECK (saldo >= 0)
);

CREATE INDEX idx_produto_codigo ON produto (codigo);

-- migrations/0002_create_idempotencia_baixa.sql
-- Guarda o resultado de uma baixa já processada, para replay idempotente
CREATE TABLE idempotencia_baixa (
    chave         VARCHAR(100) PRIMARY KEY,   -- ex: 'impressao-nota-42'
    status_http   INTEGER      NOT NULL,      -- 200, 409, 422 etc — resposta original
    resposta_json JSONB        NOT NULL,      -- corpo de resposta original, para replay exato
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_idempotencia_criado_em ON idempotencia_baixa (criado_em);

-- migrations/0003_seed_produtos.sql: catálogo de exemplo, com
-- ON CONFLICT (codigo) DO NOTHING para ser reaplicável sem efeito colateral.
```

Notas de design:
- `codigo` é o identificador de negócio informado pelo usuário e a referência
  lógica usada entre serviços — o faturamento nunca guarda o `id` interno do
  estoque.
- `idempotencia_baixa` guarda a resposta inteira em JSONB para permitir replay
  byte-a-byte sem reexecutar lógica de domínio quando a chave repetir.
- Sem job de expiração nesta fase — extensão futura razoável (limpeza por
  `criado_em`), fora do escopo atual.

### 1.2 Serviço `faturamento` — schema `faturamento`

Mesmo mecanismo da seção 1.0: SQL não qualificada, schema criado pelo runner
a partir do `search_path`.

```sql
-- migrations/0001_create_schema_e_nota.sql
CREATE SEQUENCE nota_fiscal_numero_seq START WITH 1 INCREMENT BY 1;

CREATE TABLE nota_fiscal (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    numero     BIGINT NOT NULL DEFAULT nextval('nota_fiscal_numero_seq'),
    status     VARCHAR(10) NOT NULL DEFAULT 'Aberta',
    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
    fechado_em TIMESTAMPTZ NULL,

    CONSTRAINT uq_nota_numero UNIQUE (numero),
    CONSTRAINT ck_nota_status CHECK (status IN ('Aberta', 'Fechada'))
);

ALTER SEQUENCE nota_fiscal_numero_seq OWNED BY nota_fiscal.numero;

CREATE TABLE nota_fiscal_item (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nota_id           BIGINT NOT NULL REFERENCES nota_fiscal(id) ON DELETE CASCADE,
    produto_codigo    VARCHAR(50)  NOT NULL, -- referência lógica, SEM FK para o schema estoque
    produto_descricao VARCHAR(200) NOT NULL, -- snapshot no momento da criação
    quantidade        INTEGER NOT NULL,

    CONSTRAINT ck_item_quantidade_positiva CHECK (quantidade > 0)
);

CREATE INDEX idx_item_nota_id ON nota_fiscal_item (nota_id);
CREATE INDEX idx_nota_status ON nota_fiscal (status);
```

O `nextval` da sequência também é não qualificado. O Postgres resolve esse nome
no momento do `CREATE TABLE` e guarda no `DEFAULT` uma referência direta ao
objeto (`regclass`), então a sequência amarrada à coluna é sempre a criada
segundos antes pela mesma migration, no mesmo schema — em produção ou no schema
de teste, conforme o `SET LOCAL search_path` daquela transação.

Notas de design:
- `numero` usa uma `SEQUENCE` explícita — a geração é responsabilidade do
  banco, não da aplicação, para sobreviver a requisições concorrentes sem
  duplicar número.
- `produto_descricao` é um **snapshot** capturado a partir da resposta do
  estoque no momento da criação do item, para a nota não mudar retroativamente
  se a descrição do produto for editada depois.
- Sem FK entre schemas: a existência do `produto_codigo` é validada via
  chamada HTTP síncrona ao estoque no momento de montar a nota.

---

## 2. Contratos de API REST

### 2.1 Formato único de erro

Compartilhado pelos dois serviços e consumido pelo Angular:

```json
{
  "erro": {
    "codigo": "SALDO_INSUFICIENTE",
    "mensagem": "Saldo insuficiente para o produto PARAF-001. Disponível: 3, solicitado: 5.",
    "tipo": "negocio",
    "repetivel": false
  }
}
```

- `codigo` — string estável, usada em testes e, se necessário, em lógica
  condicional no frontend.
- `mensagem` — já em português, pronta para exibição direta ao usuário, nunca
  stack trace.
- `tipo` — `"negocio"` (usuário resolve: saldo insuficiente, nota já fechada)
  ou `"sistema"` (transitório: timeout, serviço indisponível).
- `repetivel` — `true` quando vale a pena tentar de novo (timeout, 503),
  `false` quando repetir não muda o resultado (422, 409 de negócio).

### 2.2 Serviço `estoque`

| Método | Rota | Descrição | Sucesso | Erros |
| --- | --- | --- | --- | --- |
| `POST` | `/produtos` | Cria produto | `201` | `422` (validação), `409` (código duplicado) |
| `GET` | `/produtos` | Lista produtos | `200` | — |
| `GET` | `/produtos/{codigo}` | Detalhe de um produto | `200` | `404` |
| `PUT` | `/produtos/{codigo}` | Atualiza descrição/saldo | `200` | `422`, `404` |
| `DELETE` | `/produtos/{codigo}` | Remove produto | `204` | `404`, `409` (em uso) |
| `POST` | `/produtos/baixa` | Debita saldo de múltiplos produtos, atomicamente, idempotente | `200` | `422` (saldo insuficiente), `404` (produto não encontrado), `409` (chave conflitante) |

Exemplo de cadastro:

```json
// POST /produtos
{ "codigo": "PARAF-001", "descricao": "Parafuso sextavado M6", "saldo": 10 }

// 201
{ "id": 1, "codigo": "PARAF-001", "descricao": "Parafuso sextavado M6", "saldo": 10 }
```

Exemplo do endpoint de baixa (chamado pelo faturamento durante a impressão):

```json
// POST /produtos/baixa
// Header: Idempotency-Key: impressao-nota-42
{
  "itens": [
    { "codigo": "PARAF-001", "quantidade": 2 },
    { "codigo": "MART-002",  "quantidade": 1 }
  ]
}

// 200 (primeira chamada, ou replay de chave repetida)
{
  "itens": [
    { "codigo": "PARAF-001", "saldoAnterior": 10, "saldoAtual": 8 },
    { "codigo": "MART-002",  "saldoAnterior": 5,  "saldoAtual": 4 }
  ]
}

// 422 (saldo insuficiente em algum item — nenhum item é debitado)
{
  "erro": {
    "codigo": "SALDO_INSUFICIENTE",
    "mensagem": "Saldo insuficiente para o produto PARAF-001. Disponível: 1, solicitado: 2.",
    "tipo": "negocio",
    "repetivel": false
  }
}
```

### 2.3 Serviço `faturamento`

| Método | Rota | Descrição | Sucesso | Erros |
| --- | --- | --- | --- | --- |
| `POST` | `/notas` | Cria nota Aberta com itens | `201` | `422` (item inválido/inexistente), `503` (estoque indisponível para validar catálogo) |
| `GET` | `/notas` | Lista notas | `200` | — |
| `GET` | `/notas/{id}` | Detalhe da nota com itens | `200` | `404` |
| `POST` | `/notas/{id}/imprimir` | Executa a impressão — ver seção 3 | `200` | `409` (nota não está Aberta), `422` (saldo insuficiente), `503` (estoque indisponível/timeout) |
| `POST` | `/notas/interpretar` | Suporte à IA: texto livre → itens estruturados sugeridos, não persiste nada | `200` | `503`/`504` (IA indisponível — ver seção 7) |

```json
// POST /notas
{
  "itens": [
    { "produtoCodigo": "PARAF-001", "quantidade": 2 },
    { "produtoCodigo": "MART-002",  "quantidade": 1 }
  ]
}

// 201
{
  "id": 42,
  "numero": 1007,
  "status": "Aberta",
  "itens": [
    { "produtoCodigo": "PARAF-001", "produtoDescricao": "Parafuso sextavado M6", "quantidade": 2 },
    { "produtoCodigo": "MART-002",  "produtoDescricao": "Martelo de borracha",   "quantidade": 1 }
  ]
}
```

```json
// POST /notas/42/imprimir — sucesso
{ "id": 42, "numero": 1007, "status": "Fechada", "fechadoEm": "2026-09-08T14:32:10Z" }

// POST /notas/42/imprimir — estoque indisponível
{
  "erro": {
    "codigo": "ESTOQUE_INDISPONIVEL",
    "mensagem": "Não foi possível confirmar a baixa de estoque agora. Tente novamente em instantes.",
    "tipo": "sistema",
    "repetivel": true
  }
}

// POST /notas/42/imprimir — nota já fechada
{
  "erro": {
    "codigo": "NOTA_NAO_ABERTA",
    "mensagem": "Esta nota já foi fechada e não pode ser impressa novamente.",
    "tipo": "negocio",
    "repetivel": false
  }
}
```

### 2.4 Conflito de idempotência com payload divergente

Se a mesma `Idempotency-Key` chegar ao estoque com um corpo diferente do que
gerou o resultado salvo — situação anômala, já que a chave é determinística
por nota — o estoque responde `409 IDEMPOTENCY_KEY_CONFLITO`,
`tipo: "sistema"`, `repetivel: false`. Protege contra bug de cliente; não é
caminho esperado no fluxo normal.

---

## 3. Fluxo detalhado da operação de impressão

```
Angular                Faturamento                    Estoque
   │  POST /notas/42/imprimir │                              │
   ├─────────────────────────►│                              │
   │                          │ 1. SELECT nota WHERE id=42    │
   │                          │    status != 'Aberta' → 409 NOTA_NAO_ABERTA, FIM
   │                          │    status == 'Aberta' → continua
   │                          │                              │
   │                          │ 2. POST /produtos/baixa       │
   │                          │    Idempotency-Key:           │
   │                          │    impressao-nota-42          │
   │                          ├─────────────────────────────►│
   │                          │                              │ 2a. BEGIN TX
   │                          │                              │   SELECT chave em idempotencia_baixa
   │                          │                              │   existe? → devolve resultado salvo (replay)
   │                          │                              │   não existe? → valida saldo (mesma TX)
   │                          │                              │   saldo insuficiente? → ROLLBACK, 422
   │                          │                              │   ok → UPDATE saldo de todos os itens
   │                          │                              │   INSERT (chave, 200, resposta)
   │                          │                              │   COMMIT
   │                          │◄─────────────────────────────┤
   │                          │ 3. 200 → segue passo 4         │
   │                          │    422 → nota permanece Aberta, erro sobe, FIM
   │                          │    timeout/503 → retry (seção 4);
   │                          │      se esgotar → nota permanece Aberta, 503, FIM
   │                          │                              │
   │                          │ 4. UPDATE nota_fiscal          │
   │                          │    SET status='Fechada',      │
   │                          │    fechado_em=now()            │
   │                          │    WHERE id=42                │
   │◄─────────────────────────┤                              │
   │  200 Fechada  OU  erro   │                              │
```

### 3.1 Caminho de falha A — estoque indisponível antes de debitar

O faturamento chama `/produtos/baixa` e recebe timeout, conexão recusada ou
`5xx`. A política de retry (seção 4) tenta algumas vezes; se esgotar, a nota
**não** é marcada como Fechada. Resposta ao Angular: `503
ESTOQUE_INDISPONIVEL`, `repetivel: true`. Estado final: nota continua Aberta,
nenhum saldo tocado — o usuário pode tentar de novo assim que o estoque
voltar.

### 3.2 Caminho de falha B — estoque debita, mas o fechamento da nota falha depois

O passo 2 teve sucesso: o estoque debitou o saldo e gravou `(chave,
resultado)` na mesma transação. O passo 4 falha por qualquer razão no lado do
faturamento. Estado momentâneo: saldo já debitado, nota ainda Aberta.

**Recuperação via idempotência:** ao tentar imprimir de novo, o faturamento
repete o fluxo. A nota ainda está Aberta (passa no passo 1). A chamada a
`/produtos/baixa` usa a **mesma chave** `impressao-nota-42` — o estoque
encontra a chave já registrada e devolve o resultado salvo **sem debitar de
novo**. O faturamento recebe sucesso e completa o `UPDATE` para Fechada.

Resultado: saldo debitado **uma única vez**, mesmo com duas chamadas de baixa,
e a nota converge para Fechada no retry. Este é o cenário que prova, ao mesmo
tempo, o requisito obrigatório de recuperação de falha e o opcional de
idempotência — é o ponto alto do vídeo de demonstração.

---

## 4. Política de resiliência entre faturamento e estoque

Implementada como cliente HTTP interno no serviço `faturamento` (pacote
próprio, ex. `internal/estoqueclient`), usado apenas para chamar
`/produtos/baixa`.

| Parâmetro | Valor sugerido | Racional |
| --- | --- | --- |
| Timeout por tentativa | 2s | Chamada local (mesma rede docker); folgado o bastante sem travar a tela |
| Tentativas máximas | 3 (1 original + 2 retries) | Absorve reinicialização rápida do container sem multiplicar a espera |
| Backoff | Exponencial com jitter: 200ms, depois ~500ms±jitter | Evita que múltiplas impressões simultâneas martelem o estoque ao se recuperar |
| Tempo total antes de desistir | ~3s | Feedback ao usuário em poucos segundos, nunca spinner indefinido |
| Circuit breaker | Abre após 5 falhas consecutivas; 10s aberto; depois half-open | Falha imediata quando o estoque está claramente fora do ar, sem gastar os 3s de retry |

O que é **repetível**: timeout, erro de conexão, `502`/`503`/`504`
(`tipo: sistema`). O que **não é repetível**: `422 SALDO_INSUFICIENTE`,
`404 PRODUTO_NAO_ENCONTRADO`, `409 IDEMPOTENCY_KEY_CONFLITO` — o resultado não
muda com retry, repetir só atrasa o feedback.

Decisão de implementação (tomada aqui, registrada no detalhamento técnico):
`context.WithTimeout` por tentativa, retry com jitter e circuit breaker
**escritos à mão** em `internal/estoqueclient/{retry,circuitbreaker}.go`, em
vez de `sethvargo/go-retry` e `sony/gobreaker`. As duas peças são pequenas o
bastante para não pagarem uma dependência externa, e manter o `go.mod` com uma
única dependência direta (`pgx`) é um resultado que vale explicar numa
entrevista. O `WriteTimeout` do servidor HTTP de faturamento é folgadamente
maior que o tempo total do cliente de estoque, para nunca cortar a resposta no
meio de um retry.

---

## 5. Estrutura de pastas

### 5.1 Raiz do repositório

```
Korp_Teste_GustavoPinto/
├── CLAUDE.md
├── docker-compose.yml
├── .env.example                 # só variáveis que o compose realmente lê
├── .gitignore
├── README.md                    # como subir, testar e demonstrar a falha
├── DETALHAMENTO_TECNICO.md      # respostas ao checklist do enunciado
├── .github/
│   └── workflows/
│       └── ci.yml
├── docs/
│   └── plano-tecnico.md
├── frontend/
└── services/
    ├── estoque/
    └── faturamento/
```

### 5.2 Serviço Go (padrão idêntico para `estoque` e `faturamento`)

```
services/estoque/
├── go.mod
├── go.sum
├── Dockerfile
├── cmd/
│   └── api/
│       └── main.go          # bootstrap: config, conexão DB, router, graceful shutdown
├── .dockerignore             # mantém testes e fixtures fora do contexto de build
├── internal/
│   ├── httpserver/
│   │   └── handler/          # handlers HTTP
│   ├── domain/                # entidades e regras de negócio puras
│   │   ├── produto.go
│   │   └── erros.go           # ErrSaldoInsuficiente, ErrProdutoNaoEncontrado
│   ├── service/                 # orquestração de caso de uso
│   ├── repository/               # implementação com pgx
│   │   ├── produto_repository.go
│   │   └── idempotencia_repository.go
│   ├── migrate/                   # runner de migrations próprio (seção 1.0)
│   ├── testdb/                     # setup do banco de teste; nada em produção importa
│   └── apierror/                    # tradução domínio → formato único de erro HTTP
└── migrations/
    ├── embed.go                      # go:embed *.sql
    ├── 0001_create_schema_e_produto.sql
    ├── 0002_create_idempotencia_baixa.sql
    └── 0003_seed_produtos.sql
```

Um `internal/config/` chegou a ser previsto, mas não se justificou: cada
serviço lê meia dúzia de variáveis de ambiente direto em `cmd/api/main.go`, e
um pacote para isso seria indireção sem ganho.

`services/faturamento` segue a mesma estrutura, trocando `produto` por
`nota_fiscal`/`nota_fiscal_item`, e adicionando `internal/estoqueclient/`
(cliente resiliente da seção 4) e `internal/ia/` (integração da seção 7).
Camadas: **handler → service/domain → repository** — o domínio não importa
`net/http` nem driver de banco.

### 5.3 Frontend Angular

```
frontend/
├── angular.json
├── package.json
├── Dockerfile                                   # build + nginx que serve a SPA
├── nginx.conf                                   # proxy reverso /api/* (produção)
├── proxy.conf.json                              # mesmo proxy para `ng serve`
├── .dockerignore
├── src/
│   ├── styles.scss                              # tema Material 3 via mat.theme()
│   └── app/
│       ├── core/
│       │   ├── models/                        # Produto, NotaFiscal, ErroApi
│       │   ├── services/
│       │   │   ├── produto.service.ts          # base /api/estoque
│       │   │   └── nota-fiscal.service.ts      # base /api/faturamento, inclui /notas/interpretar
│       │   └── interceptors/
│       │       └── erro-http.interceptor.ts    # traduz ErroApi em mensagem uniforme
│       ├── produtos/
│       │   ├── produto-lista/
│       │   └── produto-form/
│       └── notas-fiscais/
│           ├── nota-lista/
│           ├── nota-form/
│           │   └── ia-sugestao/                 # entrada em linguagem natural
│           └── nota-detalhe/
│               └── botao-imprimir/               # estados idle/processando/sucesso/erro
```

Duas simplificações em relação ao previsto: a chamada de IA não ganhou um
`ia.service.ts` próprio — `/notas/interpretar` é um endpoint do faturamento
como qualquer outro, então mora em `nota-fiscal.service.ts`; e um diretório
`shared/` nunca foi criado, porque nenhum componente chegou a ser reaproveitado
entre as duas áreas de funcionalidade.

Os dois serviços HTTP apontam para caminhos **relativos** (`/api/estoque`,
`/api/faturamento`): nenhum host de backend entra no bundle, e é o proxy
reverso (seção 8) que resolve o destino em cada ambiente.

---

## 6. Estrutura das telas Angular

**Produtos** — `produto-lista` (tabela Material com código, descrição, saldo)
e `produto-form` (formulário reativo, validação síncrona de campos
obrigatórios e saldo ≥ 0, `ngOnInit` carrega produto em modo edição).

**Notas fiscais** — `nota-lista` (tabela com número, status em chip
colorido, data) e `nota-form`: lista de itens dinâmica via `FormArray`, cada
linha com seletor de produto por autocomplete (`debounceTime` + `switchMap`
chamando `GET /produtos`) e quantidade. O bloco `ia-sugestao` fica no topo do
formulário: campo de texto livre + botão "sugerir itens", que preenche o
`FormArray` sem nunca enviar direto ao backend de nota. `nota-detalhe` mostra
itens, status e o `botao-imprimir`.

**Botão de impressão** — máquina de estados explícita
(`idle | processando | sucesso | erro`):

- **idle** — habilitado só se `status === 'Aberta'`; desabilitado com
  `matTooltip` explicando o motivo quando Fechada.
- **processando** — `switchMap` dispara a requisição; botão desabilitado
  imediatamente (proteção contra duplo clique na própria UI, complementando a
  idempotência do backend); spinner inline.
- **sucesso** — status atualizado localmente para Fechada, saldo refletido,
  confirmação via `MatSnackBar`.
- **erro** — mensagem do contrato único exibida distinguindo visualmente
  `tipo: negocio` de `tipo: sistema`; nota permanece Aberta na tela.
- `finalize()` no pipe RxJS garante saída limpa do estado de processamento em
  qualquer desfecho, inclusive cancelamento de subscription.

---

## 7. Design da funcionalidade de IA

**Caso de uso:** criação de nota fiscal por linguagem natural.

```json
// POST /notas/interpretar
{ "texto": "3 parafusos sextavados e 2 martelos" }

// 200
{
  "itensSugeridos": [
    { "produtoCodigo": "PARAF-001", "produtoDescricao": "Parafuso sextavado M6", "quantidade": 3, "confianca": "alta" },
    { "produtoCodigo": "MART-002",  "produtoDescricao": "Martelo de borracha",   "quantidade": 2, "confianca": "alta" }
  ],
  "itensNaoReconhecidos": [
    { "textoOriginal": "5 unidades de disco de corte", "motivo": "produto não encontrado no catálogo" }
  ]
}

// 503/504 — IA indisponível ou timeout
{
  "erro": {
    "codigo": "IA_INDISPONIVEL",
    "mensagem": "Não foi possível interpretar o texto agora. Preencha os itens manualmente.",
    "tipo": "sistema",
    "repetivel": true
  }
}
```

Fluxo: o modelo recebe o texto e devolve **saída estruturada** (via tool
use / structured output do provedor). O faturamento valida cada item contra o
catálogo real
(`GET /produtos`) — produto sugerido que não existe vai para
`itensNaoReconhecidos`, nunca é inventado como novo produto. A resposta
preenche o `FormArray` no Angular; o usuário revisa e só então clica "Salvar
nota", que segue o fluxo normal de `POST /notas`.

**Degradação graciosa:** timeout de 5s (modelos de linguagem podem ser mais
lentos que uma chamada interna). Falha do provedor **nunca** bloqueia o
cadastro manual — `/notas/interpretar` apenas devolve `503`/`504` e o
formulário manual segue funcionando. Chave de API fora do repositório, via
variável de ambiente, com `.env.example` commitado e `.env` no `.gitignore` —
o repositório é público. A IA nunca tem acesso a `/produtos/baixa` nem a
qualquer escrita de saldo.

---

## 8. Orquestração via `docker-compose.yml`

O arquivo real é [`docker-compose.yml`](../docker-compose.yml) na raiz; esta
seção registra as decisões por trás dele, não uma segunda cópia a manter em
sincronia.

**Quatro serviços:** `postgres` (volume `pgdata` nomeado, para o saldo
sobreviver a `stop`/`start`), `estoque`, `faturamento` e `frontend`.

**Ordem de subida por saúde, não por sleep.** `postgres` tem healthcheck
`pg_isready`; `estoque` e `faturamento` herdam o `HEALTHCHECK` declarado em
cada Dockerfile (um `wget` no `/health`, que por sua vez faz ping no pool de
conexões). `faturamento` declara
`depends_on: {postgres: service_healthy, estoque: service_healthy}`. O
healthcheck **não é repetido no compose**: duplicá-lo criaria duas definições
para manter em sincronia, e a do compose venceria em silêncio a da imagem.

**Configuração — só variável que produz efeito.** O `.env.example` declara
exclusivamente o que o compose lê: `POSTGRES_USER`/`PASSWORD`/`DB` (única fonte
de verdade da conexão, a partir da qual as duas `DATABASE_URL` são montadas
inline, com `search_path` diferente por serviço), `IA_PROVIDER`/`IA_API_KEY`, e
as três portas de host.

**As portas de host são só o lado esquerdo do mapeamento.** `ESTOQUE_PORT`,
`FATURAMENTO_PORT` e `FRONTEND_PORT` existem para o caso de a porta já estar
ocupada na máquina de quem avalia; dentro da rede do compose os serviços ficam
fixos em 8081/8082/80. Amarrar a porta interna a essas variáveis seria uma
armadilha silenciosa: a variável mudaria a porta do app, mas não o `EXPOSE`, o
`HEALTHCHECK` do Dockerfile nem o `proxy_pass` do nginx — o container nunca
ficaria `healthy` e o `faturamento`, preso ao `service_healthy`, jamais subiria.

**Migrations e seed rodam no start de cada serviço Go**, antes do servidor HTTP
aceitar tráfego, para `docker compose up` funcionar em clone limpo sem passo
manual.

**Cenário de falha.** `depends_on` ordena apenas a **subida inicial**. Com tudo
no ar, `docker compose stop estoque` derruba só aquele container; `faturamento`
e `frontend` continuam respondendo. Roteiro do vídeo: `stop estoque` → tentar
imprimir → `503 ESTOQUE_INDISPONIVEL`, nota permanece Aberta → `start estoque`
(sem recriar volume, saldo intacto) → imprimir de novo → sucesso, saldo
debitado uma única vez. As portas 8081/8082 seguem publicadas justamente para
permitir esse roteiro com `curl` direto no serviço.

**Frontend com proxy reverso.** O container do frontend serve o bundle
estático e faz proxy de `/api/estoque/` e `/api/faturamento/` para os serviços
internos (`frontend/nginx.conf`). A SPA fala apenas com a própria origem: não
há requisição cross-origin, nem preflight, nem cabeçalho CORS em nenhum serviço
Go, nem host de backend gravado no bundle. Em desenvolvimento,
`frontend/proxy.conf.json` dá o mesmo comportamento ao `ng serve`.

---

## 9. Roadmap de implementação por fases

Sem datas fixas — a ordem é o que importa, não o calendário. Cada fase é
considerada concluída pelo teste que a prova, não por tempo decorrido.
Prioriza ter o caminho feliz fim a fim o quanto antes:

1. **Estoque isolado** — CRUD de produto, schema + migration,
   `CHECK (saldo >= 0)`, testes de domínio. Serviço mais simples, sem
   dependência externa; valida o padrão de camadas que o faturamento replica.
2. **Faturamento isolado** (sem chamar o estoque ainda) — CRUD de nota,
   itens, sequência de numeração, máquina de status Aberta/Fechada. Prova a
   numeração sequencial e a transição de status isoladamente.
3. **Integração de impressão — caminho feliz** — `POST /produtos/baixa`,
   faturamento chamando estoque antes de fechar a nota. Primeiro momento
   demonstrável de ponta a ponta.
4. **Resiliência e idempotência** — tabela de idempotência, retry/timeout/
   backoff/circuit breaker, contrato de erro único, cenário de falha testado
   manualmente com `docker compose stop estoque`. Requisito obrigatório de
   maior peso — feito logo após o caminho feliz existir.
5. **Frontend Angular** — três telas, botão de impressão com estados,
   interceptor de erro, RxJS nos pontos certos. Construído depois de os
   contratos de API estarem estáveis, para evitar retrabalho de tela.
6. **Funcionalidade de IA** — `POST /notas/interpretar`, componente
   `ia-sugestao`, degradação graciosa. Opcional e isolada: se atrasar, o
   sistema principal já está completo e entregável sem ela.
7. **Testes de ponta a ponta e Docker Compose completo** — `docker compose
   up` limpo, seed, healthchecks, testes de integração do fluxo de impressão
   (incluindo duplo clique e serviço caído). Validação final de que um clone
   limpo funciona — é o que o avaliador vai realmente rodar.
8. **Documentação e vídeo** — revisão do detalhamento técnico e gravação do
   roteiro de falha. Feita por último para descrever o sistema como ficou, não
   como foi planejado.

Os requisitos obrigatórios (fases 1–4) vêm antes de qualquer trabalho no
opcional ou no polish. Se o prazo apertar, o corte acontece nas últimas fases,
nunca nas primeiras.

---

## 10. Fluxo de commits e CI/CD

Prática transversal a todas as fases acima — não é um passo do fim.

- **Commits desde o primeiro trabalho de código.** O repositório remoto já
  existe (`gustavopinto244/Korp_Teste_GustavoPinto`); o histórico de commits
  é parte da avaliação de rastreabilidade. Cada unidade de trabalho completa e
  coerente vira um commit — não se acumula trabalho num único commit grande no
  fim. Mensagens seguem o estilo Conventional Commits (`feat:`, `fix:`,
  `test:`, `ci:`, `docs:`), descrevendo o quê e o porquê.
- **Testes acompanham a implementação, não vêm depois dela.** Cada serviço Go
  ganha testes de domínio (invariantes) e de integração (fluxo de impressão,
  incluindo os dois caminhos de falha da seção 3) no mesmo commit ou logo em
  seguida à funcionalidade que testam.
- **CI/CD via GitHub Actions desde o início do código.** Workflow
  `.github/workflows/ci.yml` rodando a cada push e pull request, com jobs
  paralelos:
  - `estoque`: `go build ./...`, `go vet ./...`, `go test ./... -p 1` contra um
    Postgres de serviço, com `TEST_DATABASE_URL_ESTOQUE` apontando para
    `search_path=estoque_test` — sem essa variável os testes de integração
    seriam pulados e o job ficaria verde sem provar nada
  - `faturamento`: idem, com `TEST_DATABASE_URL_FATURAMENTO` e
    `search_path=faturamento_test`
  - `frontend`: `npm ci`, `ng lint`, `ng build`, `ng test --watch=false`

  O pipeline fica verde desde os primeiros commits — sinal de confiabilidade
  contínua, não um selo colado no fim para a entrega.
- **Branch principal protegida por convenção**, mesmo sem regra de servidor:
  só mesclar com o CI verde.

---

## Arquivos críticos de implementação

- `services/estoque/migrations/0001_create_schema_e_produto.sql` e
  `0002_create_idempotencia_baixa.sql` — ancoram a invariante de saldo e a
  idempotência.
- `services/*/internal/migrate/migrate.go` — runner de migrations e resolução
  de schema pelo `search_path` (seção 1.0); é o que dá isolamento real de teste.
- `services/faturamento/internal/estoqueclient/` — cliente HTTP resiliente
  (seção 4).
- `services/estoque/internal/service/baixa_service.go` — validação de entrada,
  débito atômico e replay idempotente, do lado do estoque.
- `services/faturamento/internal/service/imprimir_service.go` — orquestra o
  fluxo da seção 3, coração do requisito avaliado.
- `services/*/internal/apierror/` — tradução única para o formato de erro da
  seção 2.1.
- `frontend/src/app/notas-fiscais/nota-detalhe/botao-imprimir/` — máquina de
  estados da seção 6.
- `frontend/nginx.conf` — proxy reverso que põe SPA e APIs na mesma origem
  (seção 8).
- `docker-compose.yml` — viabiliza o cenário de falha ao vivo do vídeo.
- `.github/workflows/ci.yml` — pipeline de testes ativo desde o início, com as
  variáveis `TEST_DATABASE_URL_*` definidas para a integração rodar de fato.
