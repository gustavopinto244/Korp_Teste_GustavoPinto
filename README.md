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
| Frontend | http://localhost:4200 |
| API estoque | http://localhost:8081 |
| API faturamento | http://localhost:8082 |
| Postgres | localhost:5432 |

A subida aplica as migrations e o seed de produtos de exemplo automaticamente
— não há passo manual entre `docker compose up` e um sistema utilizável.

## Testar o cenário de falha (requisito obrigatório)

```bash
# 1. crie e imprima uma nota normalmente pelo frontend ou via curl
curl -X POST http://localhost:8082/notas -H 'Content-Type: application/json' \
  -d '{"itens":[{"produtoCodigo":"PARAF-001","quantidade":1}]}'
# anote o "id" retornado

# 2. derrube o estoque
docker compose stop estoque

# 3. tente imprimir — erro claro, nota permanece Aberta
curl -X POST http://localhost:8082/notas/<id>/imprimir

# 4. religue o estoque
docker compose start estoque

# 5. imprima de novo — sucesso, saldo debitado uma única vez
curl -X POST http://localhost:8082/notas/<id>/imprimir
```

Este roteiro prova, na mesma sequência, o requisito obrigatório de
recuperação de falha e o requisito opcional de idempotência (a chave de
idempotência garante que o saldo não é debitado duas vezes mesmo com duas
chamadas de impressão para a mesma nota).

## Rodar os testes

```bash
# estoque e faturamento — cada um precisa de um Postgres acessível
cd services/estoque
go test ./... -p 1   # -p 1: testes de integração compartilham um Postgres

cd services/faturamento
go test ./... -p 1

# frontend
cd frontend
npm ci
npx ng lint
npx ng test --watch=false
```

O pipeline de CI (`.github/workflows/ci.yml`) roda os três a cada push/PR,
cada serviço Go com seu próprio Postgres de CI.

## Arquitetura

```
Angular SPA ──HTTP──> faturamento ──HTTP──> estoque
                            │                   │
                       schema faturamento  schema estoque
```

- **estoque** — dono exclusivo do saldo de produtos.
- **faturamento** — dono exclusivo da nota fiscal; para debitar saldo, chama
  o estoque via HTTP — nunca escreve no schema dele.

Detalhes completos (modelo de dados, contratos de API, fluxo de impressão,
política de resiliência): [`docs/plano-tecnico.md`](docs/plano-tecnico.md).

## Funcionalidade de IA

`POST /notas/interpretar` aceita texto livre ("3 parafusos e 2 martelos") e
devolve itens estruturados validados contra o catálogo real. Nesta versão a
interpretação usa um **mock determinístico por heurística de texto** (não é
um modelo de linguagem) atrás da interface `InterpretadorDeTexto` — ver
`services/faturamento/internal/ia/`. Para plugar um provedor real, defina
`IA_PROVIDER=claude` e `IA_API_KEY` no `.env` e implemente
`internal/ia/claude_interpretador.go` (o ponto de extensão já existe, hoje
retorna erro explícito de "não implementado").
