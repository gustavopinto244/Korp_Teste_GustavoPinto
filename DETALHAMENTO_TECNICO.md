# Detalhamento técnico

Respostas item a item do checklist exigido pelo enunciado do desafio. Cada
afirmação abaixo aponta para um arquivo real do repositório.

## Ciclos de vida do Angular utilizados

- **`ngOnInit`** — usado nos cinco componentes de tela: `produto-lista.ts` e
  `nota-lista.ts` (carregam a listagem ao entrar), `produto-form.ts` (carrega o
  produto existente quando a rota é de edição), `nota-detalhe.ts` (carrega a
  nota pelo id da rota) e `nota-form.ts` (inicializa o array de sugestões do
  autocomplete alinhado ao `FormArray`).
- **Nenhum `ngOnDestroy`** — a decisão é deliberada e vale a pena ser explícita,
  porque as dez inscrições RxJS do projeto se dividem em dois casos distintos:
  - **Um fluxo de vida longa**, o `valueChanges` do autocomplete de produto em
    `nota-form.ts`. É um `Subject` que nunca completa sozinho: sem cleanup, a
    inscrição sobreviveria ao componente. Esse é o único que precisa de
    encerramento explícito e o único que usa
    `takeUntilDestroyed(this.destroyRef)` (`@angular/core/rxjs-interop`), com o
    `DestroyRef` injetado no campo da classe — a linha do `FormArray` também é
    criada a partir de handlers de evento (adicionar item, aceitar sugestão da
    IA), fora de contexto de injeção, onde `takeUntilDestroyed()` sem argumento
    lançaria `NG0203`.
  - **Nove inscrições de vida curta**, todas em observables do `HttpClient`
    (`produto-lista.ts` ×2, `produto-form.ts` ×2, `nota-lista.ts`,
    `nota-detalhe.ts`, `nota-form.ts` no submit, `ia-sugestao.ts` e
    `botao-imprimir.ts`). Um observable do `HttpClient` emite no máximo uma vez
    e completa, o que descarta a inscrição por conta própria — não há
    vazamento de subscription a prevenir, e um `ngOnDestroy` só para
    `unsubscribe` seria cerimônia sem efeito.

  Vale registrar o limite honesto dessa escolha: uma inscrição de vida curta
  não é cancelada quando o componente é destruído no meio da requisição — o
  callback ainda roda contra um componente morto. É inofensivo aqui porque
  todos esses callbacks apenas escrevem em `signal`s do próprio componente ou
  abrem um `MatSnackBar`, nunca tocam o DOM diretamente. Se algum passasse a
  fazer navegação ou efeito colateral externo, o cancelamento deixaria de ser
  opcional. Os dois fluxos com indicador de progresso (`botao-imprimir.ts` e
  `ia-sugestao.ts`) já usam `finalize()`, que desliga o estado de
  processamento inclusive no cancelamento.
- `ngOnChanges` não foi necessário: nenhum componente depende de reagir a
  mudança de `@Input()` depois da criação (os dados de edição chegam uma vez,
  via parâmetro de rota, resolvidos em `ngOnInit`).

## Uso da biblioteca RxJS

Sim, usada em pontos concretos, não decorativos:

- **`debounceTime(300)` + `switchMap`** — autocomplete de produto no
  formulário de nota (`nota-form.ts`): evita uma requisição por tecla digitada
  e cancela a busca anterior se uma nova busca começar antes da resposta
  chegar.
- **`catchError`** — usado em dois lugares com propósitos diferentes:
  no interceptor HTTP (`erro-http.interceptor.ts`), para traduzir qualquer
  erro de resposta no formato único `ErroApi` antes de chegar ao componente;
  e na busca de produtos do autocomplete (`nota-form.ts`), para que uma falha
  de rede na busca não quebre o fluxo de digitação — devolve lista vazia em
  vez de propagar o erro.
- **`finalize`** — usado no botão de impressão (`botao-imprimir.ts`) e no
  bloco de sugestão por IA (`ia-sugestao.ts`) para garantir que o indicador de
  processamento é desligado sempre, tanto no sucesso quanto no erro.
- **`takeUntilDestroyed`** — encerra o único fluxo de vida longa da aplicação
  junto com o componente (ver seção anterior).
- O `HttpInterceptorFn` em si é a composição de um `Observable` (`next(req)`)
  com esses operadores — é o mecanismo central de tratamento de erro de toda
  a aplicação.

## Outras bibliotecas utilizadas e finalidade

Apenas o ecossistema oficial do Angular, sem dependências de terceiros para
lógica de negócio:

| Biblioteca | Finalidade |
| --- | --- |
| `@angular/forms` | Formulários reativos (`ReactiveFormsModule`, `FormArray` para múltiplos itens de nota) |
| `@angular/router` | Navegação entre telas, roteamento 100% `loadComponent` (lazy) em `app.routes.ts` |
| `@angular/cdk` | Base do Angular Material (overlay, a11y) |
| `rxjs` | Composição assíncrona — ver seção acima |

Ferramentas de desenvolvimento (não vão para o bundle de produção):
`@angular-eslint`/`eslint` para lint, `vitest` como test runner (escolha
padrão do `ng new` nesta versão do Angular CLI, no lugar de Karma),
`prettier` para formatação.

## Bibliotecas de componentes visuais

**Angular Material** (`@angular/material`), com tema **customizado via
Sass**, não um dos temas pré-construídos: `angular.json` carrega apenas
`src/styles.scss`, que aplica o mixin `mat.theme()` do Material 3 sobre
`mat.$azure-palette` (primária) e `mat.$blue-palette` (terciária), com
tipografia Roboto e densidade 0. As animações vêm de `provideAnimationsAsync()`
em `app.config.ts`. O mesmo arquivo define, por variáveis CSS do Material, as
cores dos snackbars de erro de negócio, erro de sistema e sucesso — a
diferenciação visual do contrato único de erro.

Componentes usados: tabelas (`mat-table`), formulários (`mat-form-field`,
`matInput`, `mat-error`), autocomplete de produto (`mat-autocomplete` +
`mat-option` — não há `mat-select` no projeto: a seleção é sempre por busca
com filtro), botões (`mat-button`, `mat-flat-button`, `mat-stroked-button`,
`mat-icon-button`), ícones (`mat-icon`), barra de navegação (`mat-toolbar`),
indicador de processamento (`mat-spinner`), feedback (`MatSnackBar`),
`matTooltip` explicando por que o botão de impressão está desabilitado, e o
status da nota como `mat-chip` colorido.

## Gerenciamento de dependências em Go

Go Modules padrão (`go.mod`/`go.sum`), um módulo por microsserviço — não há
módulo compartilhado entre `estoque` e `faturamento`, para manter cada um
deployável isoladamente. Cada serviço tem **uma única dependência direta**:

```
github.com/jackc/pgx/v5 v5.11.0
```

As demais entradas do `go.mod` (`pgpassfile`, `pgservicefile`, `puddle/v2`,
`golang.org/x/sync`, `golang.org/x/text`) estão marcadas `// indirect`: são
transitivas do próprio `pgx`, não escolhas do projeto. Deliberadamente não
foram adicionadas bibliotecas de migration (`golang-migrate`), retry ou
circuit breaker (`sony/gobreaker`, `sethvargo/go-retry`) — essas três peças
foram implementadas à mão (`internal/migrate`, `internal/estoqueclient/retry.go`,
`internal/estoqueclient/circuitbreaker.go` no faturamento) por serem simples
o bastante para não justificar uma dependência externa.

## Frameworks utilizados em Go

**Nenhum framework web.** Os dois serviços usam apenas `net/http` da
biblioteca padrão, com `http.ServeMux` (Go 1.22+, que já roteia por método
HTTP e path, ex. `mux.HandleFunc("POST /produtos/baixa", ...)` em
`internal/httpserver/router.go`). Decisão deliberada: o volume de rotas de
cada serviço (meia dúzia de endpoints, incluindo `/health`) não justifica Gin,
Echo ou Chi, e evitar um framework mantém o `go.mod` com a única dependência
direta citada acima.

## Tratamento de erros e exceções no backend

Estratégia única, replicada nos dois serviços:

1. **Erros de domínio são tipos Go**, não strings soltas —
   `internal/domain/erros.go` em cada serviço declara sentinelas
   (`ErrProdutoNaoEncontrado`, `ErrNotaNaoAberta` etc.) e, quando o erro
   carrega dados variáveis (ex. `ErrSaldoInsuficiente{Codigo, Disponivel,
   Solicitado}`), um tipo struct com método `Error()` e `Is()` para permitir
   `errors.Is` idiomático mesmo com valores diferentes por instância.
2. **Contexto ao propagar** — erros são envolvidos com `fmt.Errorf("...: %w",
   err)` ao subir de camada (repository → service → handler), preservando a
   cadeia original para log, sem perder o tipo para `errors.Is`/`errors.As`.
3. **Tradução para HTTP numa única camada, na borda** — `internal/apierror`
   em cada serviço mapeia cada erro de domínio para o formato único de erro:
   ```json
   { "erro": { "codigo": "...", "mensagem": "...", "tipo": "negocio|sistema", "repetivel": true|false } }
   ```
   Nenhum handler chama `http.Error` diretamente; todos passam pelo mesmo
   `apierror.EscreverErro`.
4. **Erros de negócio vs. erros de sistema** — o campo `tipo` distingue o que
   o usuário resolve (saldo insuficiente, nota já fechada) do que é
   transitório (estoque indisponível), e `repetivel` diz se vale a pena tentar
   de novo. É essa distinção que permite ao cliente resiliente do faturamento
   (`internal/estoqueclient`) decidir se repete uma chamada ou desiste na
   primeira resposta.
5. **Erro inesperado tem uma resposta só, e nunca some** — o caso `default` do
   mapeamento é idêntico nos dois serviços: `500` com
   `codigo: "ERRO_INTERNO"`, a mesma mensagem
   ("Ocorreu um erro inesperado. Tente novamente em instantes."),
   `tipo: "sistema"` e `repetivel: true`. Idêntico de propósito: o frontend
   trata o mesmo `codigo` com o mesmo significado, venha de qual serviço vier.
   Como o corpo nunca carrega a causa (o contrato manda mensagem pronta para o
   usuário, nunca stack trace), `EscreverErro` **loga o erro original** sempre
   que o status é 500 — sem isso um 500 seria indebugável, com o erro
   descartado silenciosamente.
6. **Validação de entrada antes do banco** — `POST /produtos/baixa` rejeita
   com `422 VALIDACAO` lista de itens vazia, código em branco e quantidade
   `<= 0` (`internal/service/baixa_service.go`). Não é validação decorativa:
   uma quantidade negativa passaria ilesa pela checagem de saldo insuficiente
   (`saldoAnterior < quantidade` nunca é verdadeira para negativos) e o débito
   acabaria *creditando* saldo — algo que o `CHECK (saldo >= 0)` do banco não
   barra, porque o valor sobe.
7. **Atomicidade via transação** — o endpoint de baixa do estoque
   (`internal/service/baixa_service.go`) valida saldo de todos os itens e só
   então debita, dentro de uma única transação Postgres — erro de negócio em
   qualquer item provoca rollback completo, nunca débito parcial.

## Comunicação frontend ↔ API (e o que aconteceu com o CORS)

Não há CORS no sistema porque **não há requisição cross-origin**. A SPA usa
apenas caminhos relativos (`/api/estoque`, `/api/faturamento` — ver
`core/services/*.service.ts`), e o nginx que serve o bundle faz proxy reverso
para os serviços na rede interna do compose (`frontend/nginx.conf`); em
desenvolvimento, `ng serve` faz o equivalente via `frontend/proxy.conf.json`,
registrado em `angular.json`.

A alternativa seria emitir cabeçalhos `Access-Control-Allow-*` nos dois
serviços Go. Proxy foi preferido por três razões concretas: (a) nenhum host de
backend fica gravado no bundle de produção — o mesmo artefato roda em qualquer
ambiente; (b) os serviços Go não ganham conhecimento de origem de navegador,
que é preocupação de borda, não de domínio; (c) some a categoria inteira de
falha "funciona no `curl`, quebra no navegador", inclusive preflight `OPTIONS`.

## LINQ

**Não aplicável.** O backend deste projeto foi implementado em Go, não em
C# — LINQ é específico do .NET e não existe no ecossistema Go. O enunciado
condicionava esse item a uma implementação em C#; como a escolha de stack
recaiu sobre Go (também permitido pelo enunciado), este item é registrado
aqui como não aplicável em vez de omitido.

## Isolamento de schema e migrations

Cada serviço é dono exclusivo de um schema Postgres e **nada na sua SQL é
qualificado com o nome do schema**. O runner próprio (`internal/migrate`)
deriva o schema alvo do `search_path` da conexão, cria-o se preciso e aplica
cada migration dentro de uma transação com `SET LOCAL search_path`. A tabela de
controle `schema_migrations` vive dentro do schema do serviço, não em `public`.

Isso não é purismo: é o que permite a suíte de integração rodar num schema
descartável (`estoque_test`, `faturamento_test`) derivado do próprio DSN, sem
tocar nos dados da demo nem numa tabela de controle compartilhada com o outro
microsserviço. A suíte recusa rodar se o `search_path` não terminar em
`_test`, já que ela derruba o schema inteiro antes de cada execução — ver
[`README.md`](README.md#rodar-os-testes).

## Requisitos obrigatórios — onde estão implementados

- **Arquitetura de microsserviços** — `services/estoque` e
  `services/faturamento`, schemas separados, comunicação só por HTTP.
- **Tratamento de falhas** — `services/faturamento/internal/service/imprimir_service.go`
  (fluxo completo) + `internal/estoqueclient/{retry,circuitbreaker}.go`.
  Roteiro de demonstração em [`README.md`](README.md#testar-o-cenário-de-falha-requisito-obrigatório).
- **Conexão real com banco de dados** — PostgreSQL real via `pgx/v5`,
  persistência verificada sobrevivendo a `docker compose stop`/`start`.

## Requisitos opcionais escolhidos

- **Idempotência** — chave determinística `impressao-nota-{id}`, tabela
  `idempotencia_baixa` no schema do estoque, replay exato em chave repetida
  (`services/estoque/internal/service/baixa_service.go`).
- **Inteligência Artificial** — `POST /notas/interpretar`, mock determinístico
  documentado como tal em `services/faturamento/internal/ia/mock_interpretador.go`,
  com ponto de extensão para um provedor real em `claude_interpretador.go`.
