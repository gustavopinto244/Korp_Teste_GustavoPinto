# Detalhamento técnico

Respostas item a item do checklist exigido pelo enunciado do desafio. Cada
afirmação abaixo aponta para um arquivo real do repositório.

## Ciclos de vida do Angular utilizados

- **`ngOnInit`** — usado em todos os componentes que precisam carregar dados
  ao entrar em tela: `produto-lista.ts`, `produto-form.ts` (carrega o produto
  existente quando a rota é de edição), `nota-lista.ts` e `nota-detalhe.ts`
  (carrega a nota pelo id da rota).
- Nenhum componente precisou de `ngOnDestroy` manual: a única inscrição RxJS
  fora de fluxo declarativo de template (o autocomplete de produto em
  `notas-fiscais/nota-form/nota-form.ts`) usa o operador
  `takeUntilDestroyed()` (`@angular/core/rxjs-interop`), que encerra a
  inscrição automaticamente quando o componente é destruído — mais idiomático
  no Angular atual do que implementar `OnDestroy` à mão.
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
- O `HttpInterceptorFn` em si é a composição de um `Observable` (`next(req)`)
  com esses operadores — é o mecanismo central de tratamento de erro de toda
  a aplicação.

## Outras bibliotecas utilizadas e finalidade

Apenas o ecossistema oficial do Angular, sem dependências de terceiros para
lógica de negócio:

| Biblioteca | Finalidade |
| --- | --- |
| `@angular/forms` | Formulários reativos (`ReactiveFormsModule`, `FormArray` para múltiplos itens de nota) |
| `@angular/router` | Navegação entre telas, roteamento com `loadComponent` (lazy) |
| `@angular/cdk` | Base do Angular Material (overlay, a11y) |
| `rxjs` | Composição assíncrona — ver seção acima |

Ferramentas de desenvolvimento (não vão para o bundle de produção):
`@angular-eslint`/`eslint` para lint, `vitest` como test runner (escolha
padrão do `ng new` nesta versão do Angular CLI, no lugar de Karma),
`prettier` para formatação.

## Bibliotecas de componentes visuais

**Angular Material** (`@angular/material`), tema pré-construído `azure-blue`
com tipografia e animações habilitadas
(`provideAnimationsAsync()` em `app.config.ts`). Usado para: tabelas
(`mat-table`), formulários (`mat-form-field`, `mat-input`, `mat-select`),
autocomplete (`mat-autocomplete`), indicador de processamento
(`mat-progress-spinner`), feedback (`MatSnackBar`) e status da nota como
`mat-chip` colorido.

## Gerenciamento de dependências em Go

Go Modules padrão (`go.mod`/`go.sum`), um módulo por microsserviço — não há
módulo compartilhado entre `estoque` e `faturamento`, para manter cada um
deployável isoladamente. Cada serviço tem **uma única dependência direta**:

```
github.com/jackc/pgx/v5 v5.11.0
```

As demais entradas do `go.mod` (`pgpassfile`, `pgservicefile`, `puddle/v2`,
`golang.org/x/sync`, `golang.org/x/text`) são transitivas do próprio `pgx`,
não escolhas do projeto. Deliberadamente não foram adicionadas bibliotecas de
migration (`golang-migrate`), retry ou circuit breaker (`sony/gobreaker`,
`sethvargo/go-retry`) — essas três peças foram implementadas à mão
(`internal/migrate`, `internal/estoqueclient/retry.go`,
`internal/estoqueclient/circuitbreaker.go` no faturamento) por serem simples
o bastante para não justificar uma dependência externa.

## Frameworks utilizados em Go

**Nenhum framework web.** Os dois serviços usam apenas `net/http` da
biblioteca padrão, com `http.ServeMux` (Go 1.22+, que já roteia por método
HTTP e path, ex. `mux.HandleFunc("POST /produtos/baixa", ...)` em
`internal/httpserver/router.go`). Decisão deliberada: o volume de rotas de
cada serviço (5–6 endpoints) não justifica Gin, Echo ou Chi, e evitar um
framework mantém o `go.mod` com a única dependência direta citada acima.

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
5. **Atomicidade via transação** — o endpoint de baixa do estoque
   (`internal/service/baixa_service.go`) valida saldo de todos os itens e só
   então debita, dentro de uma única transação Postgres — erro de negócio em
   qualquer item provoca rollback completo, nunca débito parcial.

## LINQ

**Não aplicável.** O backend deste projeto foi implementado em Go, não em
C# — LINQ é específico do .NET e não existe no ecossistema Go. O enunciado
condicionava esse item a uma implementação em C#; como a escolha de stack
recaiu sobre Go (também permitido pelo enunciado), este item é registrado
aqui como não aplicável em vez de omitido.

## Requisitos obrigatórios — onde estão implementados

- **Arquitetura de microsserviços** — `services/estoque` e
  `services/faturamento`, bancos/schemas separados, comunicação só por HTTP.
- **Tratamento de falhas** — `services/faturamento/internal/service/imprimir_service.go`
  (fluxo completo) + `internal/estoqueclient/{retry,circuitbreaker}.go`.
  Roteiro de demonstração em [`README.md`](README.md#testar-o-cenário-de-falha-requisito-obrigatório).
- **Conexão real com banco de dados** — PostgreSQL real via `pgx/v5`,
  persistência verificada sobrevivendo a `docker compose stop`/`start`.

## Requisitos opcionais escolhidos

- **Idempotência** — chave determinística `impressao-nota-{id}`, tabela
  `estoque.idempotencia_baixa`, replay exato em chave repetida
  (`services/estoque/internal/service/baixa_service.go`).
- **Inteligência Artificial** — `POST /notas/interpretar`, mock determinístico
  documentado como tal em `services/faturamento/internal/ia/mock_interpretador.go`,
  com ponto de extensão para um provedor real em `claude_interpretador.go`.
