# Korp_Teste_GustavoPinto — contexto do projeto

Desafio técnico da Korp: **sistema de emissão de notas fiscais**.

Este arquivo é o ponto de entrada de qualquer sessão e de qualquer agente.
Se algo aqui contradiz o que foi dito em outro lugar, este arquivo vence.
Se este arquivo estiver errado, corrija-o no mesmo commit que prova o erro.

---

## 1. Entrega — o que a Korp vai receber

| Item | Detalhe |
| --- | --- |
| Prazo | 7 dias corridos a partir do recebimento do desafio |
| Repositório | Público no GitHub, nome exato `Korp_Teste_GustavoPinto` |
| Vídeo | Telas + funcionalidades + detalhamento técnico, hospedado em nuvem |
| E-mail | `rh@korp.com.br` com link do repo + link do vídeo + detalhamento técnico |

**A entrega é avaliada por três artefatos, não um.** O código é apenas um deles.
O vídeo e o detalhamento técnico têm peso próprio e são cobrados item a item —
veja a seção 5.

---

## 2. Stack decidida

| Camada | Escolha | Observação |
| --- | --- | --- |
| Frontend | Angular | Obrigatório pelo enunciado |
| Backend | **Go** nos dois microsserviços | O enunciado permitia Go ou C#; Go foi escolhido |
| Banco | PostgreSQL | Um schema por serviço, sem tabela compartilhada |
| Orquestração local | Docker Compose | O demo do vídeo depende de subir tudo com um comando |

**Consequência direta da escolha de Go:** o detalhamento técnico deve declarar
explicitamente que o item de **LINQ não se aplica** (é específico de C#) e, em
troca, responder o item de **gerenciamento de dependências em Go** (go modules).
Não deixe o item de LINQ sem resposta — silêncio parece esquecimento.

---

## 3. Arquitetura

Dois microsserviços, conforme exigido. Cada um dono do seu dado.

```
┌─────────────────┐
│   Angular SPA   │
└────────┬────────┘
         │ HTTP
    ┌────┴─────────────────────┐
    ▼                          ▼
┌───────────────┐      ┌───────────────┐
│  faturamento  │─────▶│    estoque    │
│  notas fiscais│ HTTP │ produtos/saldo│
└───────┬───────┘      └───────┬───────┘
        ▼                      ▼
   schema faturamento     schema estoque
```

- **estoque** — cadastro de produtos e controle de saldo. Dono do saldo.
  Ninguém mais escreve saldo.
- **faturamento** — cadastro de notas fiscais, numeração sequencial, status.
  Dono da nota. Para baixar saldo, **chama o estoque** — nunca escreve no schema
  do estoque.

---

## 4. Regras de negócio

### Produto
- Campos obrigatórios: `código`, `descrição`, `saldo`.
- Saldo nunca fica negativo. Essa é uma invariante do serviço de estoque, não
  uma validação de tela.

### Nota fiscal
- Campos obrigatórios: `numeração sequencial`, `status`, `itens` (produto +
  quantidade, múltiplos).
- Numeração é **sequencial** — gerada pelo banco, nunca pela aplicação, para
  sobreviver a requisições concorrentes.
- Status inicial ao criar: **Aberta**. Único destino: **Fechada**.

### Impressão — a operação central
É onde o desafio realmente é avaliado. Ela cruza os dois microsserviços.

1. Só é permitida se o status for **Aberta**. Qualquer outro status é rejeitado.
2. A tela exibe **indicador de processamento** enquanto a operação corre.
3. Ao final com sucesso: status vira **Fechada** e o saldo de cada produto é
   **debitado** pela quantidade da nota (saldo 10, nota usa 2 → saldo 8).
4. Se qualquer etapa falhar: a nota **permanece Aberta**, nenhum saldo é
   debitado pela metade, e o usuário recebe um erro compreensível.

O passo 4 não é detalhe: o enunciado exige explicitamente um cenário de falha de
microsserviço com recuperação e feedback ao usuário.

---

## 5. O checklist do detalhamento técnico

O enunciado lista perguntas nominais que **precisam ser respondidas** no vídeo e
no documento. Trate como critério de aceite, não como redação livre:

- [ ] Quais ciclos de vida do Angular foram utilizados
- [ ] Se RxJS foi usado e **como**
- [ ] Quais outras bibliotecas e para qual finalidade
- [ ] Quais bibliotecas de componentes visuais
- [ ] Como foi feito o gerenciamento de dependências em Go
- [ ] Quais frameworks Go foram utilizados
- [ ] Como erros e exceções são tratados no backend
- [ ] LINQ — **declarar como não aplicável**, pois o backend é Go

Regra de ouro: cada resposta precisa apontar para código que existe. Não afirme
que um ciclo de vida foi usado se ele não está no repositório.

---

## 6. Requisitos obrigatórios e opcionais

**Obrigatórios** (sem eles a entrega está incompleta):
1. Arquitetura de microsserviços — mínimo dois. ✔ estoque + faturamento
2. Tratamento de falhas — um serviço cai, o sistema se recupera e informa o usuário
3. Conexão real com banco de dados — persistência física

**Opcionais escolhidos** (o diferencial da entrega):
- **Idempotência** — clicar imprimir duas vezes não debita o estoque duas vezes
- **Inteligência Artificial** — uma funcionalidade real do sistema usando IA

**Opcional deliberadamente fora de escopo:** tratamento de concorrência.
Não foi escolhido. Se sobrar tempo no fim, reavalie — mas não comece por ele.

---

## 7. Ecossistema de agentes

Os agentes vivem em `.claude/agents/`. Quatro camadas:

| Camada | Agentes | Papel |
| --- | --- | --- |
| Construção | `angular-dev`, `go-estoque`, `go-faturamento`, `ai-feature` | Escrevem funcionalidade |
| Transversal | `resilience-engineer`, `devops-infra` | Atravessam serviços |
| Garantia | `test-engineer`, `code-reviewer` | Verificam antes de entregar |
| Entrega | `tech-writeup`, `delivery-auditor` | Produzem o que a Korp lê e assiste |

Regra de fronteira: **nenhum agente de construção escreve no domínio do outro.**
`go-faturamento` não altera saldo; `go-estoque` não conhece nota fiscal. Quando
uma tarefa cruza essa linha, ela pertence ao `resilience-engineer`.
