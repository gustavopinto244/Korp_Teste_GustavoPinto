# Sistema de Emissão de Notas Fiscais

Desafio técnico Korp. Angular + dois microsserviços em Go (`estoque` e
`faturamento`) + PostgreSQL, orquestrados via Docker Compose.

O desenho de arquitetura completo está em [`docs/plano-tecnico.md`](docs/plano-tecnico.md).
O detalhamento técnico exigido pelo enunciado (ciclos de vida, RxJS,
bibliotecas, dependências, tratamento de erros) está em
[`DETALHAMENTO_TECNICO.md`](DETALHAMENTO_TECNICO.md).

## Subir o sistema

Requer Docker e Docker Compose.

```bash
cp .env.example .env
docker compose up --build
```

Serviços expostos:

| Serviço | URL |
| --- | --- |
| Frontend (SPA + proxy das APIs) | http://localhost:4200 |
| API estoque | http://localhost:8081 |
| API faturamento | http://localhost:8082 |
| Postgres | localhost:5432 |

As três portas de host são configuráveis pelo `.env` (`ESTOQUE_PORT`,
`FATURAMENTO_PORT`, `FRONTEND_PORT`) e mudam **apenas** o lado host do
mapeamento — dentro da rede do compose os serviços permanecem em 8081, 8082 e
80, que é o que mantém healthchecks e proxy válidos.

A subida aplica as migrations e o seed de produtos de exemplo automaticamente
— não há passo manual entre `docker compose up` e um sistema utilizável.

## Testar o cenário de falha (requisito obrigatório)

```bash
# 1. crie uma nota normalmente pelo frontend ou via curl
curl -X POST http://localhost:8082/notas -H 'Content-Type: application/json' \
  -d '{"itens":[{"produtoCodigo":"PARAF-001","quantidade":1}]}'
# anote o "id" retornado

# 2. confira o saldo antes de imprimir
curl http://localhost:8081/produtos/PARAF-001

# 3. derrube o estoque
docker compose stop estoque

# 4. tente imprimir — 503 ESTOQUE_INDISPONIVEL, nota permanece Aberta
curl -i -X POST http://localhost:8082/notas/<id>/imprimir

# 5. religue o estoque
docker compose start estoque

# 6. imprima — sucesso, nota Fechada, saldo debitado
curl -X POST http://localhost:8082/notas/<id>/imprimir

# 7. imprima de novo — 409 NOTA_NAO_ABERTA, e o saldo NÃO cai outra vez
curl -i -X POST http://localhost:8082/notas/<id>/imprimir
curl http://localhost:8081/produtos/PARAF-001
```

Este roteiro prova, na mesma sequência, o requisito obrigatório de
recuperação de falha e o requisito opcional de idempotência: a chave
determinística `impressao-nota-{id}` garante que o saldo é debitado uma única
vez, mesmo que a impressão seja tentada várias vezes.

As portas 8081/8082 continuam publicadas justamente para permitir este
roteiro com `curl` direto no serviço. As mesmas chamadas funcionam através do
proxy do frontend, que é o caminho que o navegador usa:

```bash
curl http://localhost:4200/api/estoque/produtos
curl http://localhost:4200/api/faturamento/notas
```

## Rodar os testes

Os testes de integração — que são o ponto forte da suíte: cobrem falha do
estoque, replay idempotente, atomicidade do débito, transição de status,
numeração sequencial e conflito de chave — **só rodam com um Postgres de teste
configurado por variável de ambiente**. Sem ela, `go test` marca esses testes
como `t.Skip` e imprime `ok` em poucos milissegundos por pacote: passa a
impressão de suíte verde sem ter exercitado banco nenhum.

```bash
# 1. um Postgres dedicado, em porta diferente da do compose (5432)
docker run -d --name korp-pg-test \
  -e POSTGRES_USER=korp -e POSTGRES_PASSWORD=korp -e POSTGRES_DB=korp_test \
  -p 5561:5432 postgres:16

# 2. estoque
cd services/estoque
export TEST_DATABASE_URL_ESTOQUE='postgres://korp:korp@localhost:5561/korp_test?sslmode=disable&search_path=estoque_test'
go test ./... -p 1   # -p 1: os pacotes compartilham um Postgres e fazem DROP/CREATE SCHEMA

# 3. faturamento
cd ../faturamento
export TEST_DATABASE_URL_FATURAMENTO='postgres://korp:korp@localhost:5561/korp_test?sslmode=disable&search_path=faturamento_test'
go test ./... -p 1

# 4. limpeza
docker rm -f korp-pg-test
```

Cada suíte **derruba e recria o schema inteiro** apontado pelo `search_path`
do DSN antes de aplicar as migrations. Por isso há uma trava: se o
`search_path` não terminar em `_test`, os testes falham imediatamente com uma
mensagem explicando o motivo, em vez de apagar o schema de produção — apontar
a variável para o banco do `docker compose` não destrói a demo.

O mesmo mecanismo garante que os dois serviços não se atrapalhem: cada um
opera exclusivamente dentro do seu schema (`estoque_test`,
`faturamento_test`), inclusive a tabela de controle de migrations. Por isso um
único container Postgres serve às duas suítes.

Frontend (não precisa de banco):

```bash
cd frontend
npm ci
npx ng lint
npx ng test --watch=false
```

O pipeline de CI (`.github/workflows/ci.yml`) roda os três a cada push/PR,
cada serviço Go com seu próprio Postgres de CI e as variáveis
`TEST_DATABASE_URL_*` já definidas — ou seja, a integração roda de verdade lá.

## Arquitetura

```
navegador ──> nginx (frontend) ──/api/faturamento──> faturamento ──HTTP──> estoque
                                 ──/api/estoque────> estoque         │          │
                                                                schema      schema
                                                              faturamento   estoque
```

- **estoque** — dono exclusivo do saldo de produtos.
- **faturamento** — dono exclusivo da nota fiscal; para debitar saldo, chama
  o estoque via HTTP — nunca escreve no schema dele.
- **frontend** — a SPA usa caminhos relativos (`/api/estoque/...`,
  `/api/faturamento/...`) e nunca conhece o host de um backend. Em produção o
  nginx do container faz o proxy reverso (`frontend/nginx.conf`); em
  desenvolvimento, o `ng serve` faz o mesmo via `frontend/proxy.conf.json`.
  Como tudo sai da mesma origem, **não existe requisição cross-origin no
  sistema** e nenhum serviço Go emite cabeçalho CORS.

Detalhes completos (modelo de dados, contratos de API, fluxo de impressão,
política de resiliência): [`docs/plano-tecnico.md`](docs/plano-tecnico.md).

## Funcionalidade de IA

`POST /notas/interpretar` aceita texto livre ("3 parafusos e 2 martelos") e
devolve itens estruturados validados contra o catálogo real, para o usuário
conferir antes de gravar a nota. Duas implementações atendem a mesma
interface `InterpretadorDeTexto` (`services/faturamento/internal/ia/`),
escolhidas por `IA_PROVIDER`:

| `IA_PROVIDER` | O que roda |
| --- | --- |
| `mock` (padrão) | heurística determinística local por casamento de texto — **não** é um modelo de linguagem |
| `claude` | Messages API da Anthropic pelo SDK oficial, com saída estruturada por *tool use* |

Para ligar a IA real, no `.env`:

```bash
IA_PROVIDER=claude
IA_API_KEY=sk-ant-...        # nunca comite esta chave; .env está no .gitignore
IA_MODEL=                    # opcional; vazio = claude-opus-5
```

Sem `IA_API_KEY` o serviço cai no mock de propósito, em vez de falhar em toda
requisição. Em qualquer falha do provedor — timeout, erro de rede, resposta
inesperada — o endpoint responde `503 IA_INDISPONIVEL`, registra o motivo no
log e a tela segue funcionando para cadastro manual: a IA é apoio, nunca
caminho obrigatório.

Um código de produto que o modelo invente nunca chega à tela: a resposta é
conciliada contra o catálogo vindo do estoque, e o que não casa vira "item
não reconhecido".
