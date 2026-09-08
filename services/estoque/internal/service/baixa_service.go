package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
)

// ItemBaixa representa um item da requisição de baixa: produto +
// quantidade a debitar.
type ItemBaixa struct {
	Codigo     string `json:"codigo"`
	Quantidade int    `json:"quantidade"`
}

// RequisicaoBaixa é o corpo de POST /produtos/baixa.
type RequisicaoBaixa struct {
	Itens []ItemBaixa `json:"itens"`
}

// ItemBaixaResultado é o resultado da baixa de um item específico.
type ItemBaixaResultado struct {
	Codigo        string `json:"codigo"`
	SaldoAnterior int    `json:"saldoAnterior"`
	SaldoAtual    int    `json:"saldoAtual"`
}

// RespostaBaixa é o corpo de resposta de sucesso de POST /produtos/baixa.
type RespostaBaixa struct {
	Itens []ItemBaixaResultado `json:"itens"`
}

// BaixaService orquestra a baixa atômica e idempotente de saldo de
// múltiplos produtos — o caso de uso central do serviço de estoque, chamado
// pelo faturamento durante a impressão de uma nota fiscal.
type BaixaService struct {
	produtoRepo *repository.ProdutoRepository
	idempRepo   *repository.IdempotenciaRepository
}

// NovoBaixaService cria um BaixaService sobre os repositórios informados.
func NovoBaixaService(produtoRepo *repository.ProdutoRepository, idempRepo *repository.IdempotenciaRepository) *BaixaService {
	return &BaixaService{produtoRepo: produtoRepo, idempRepo: idempRepo}
}

// Processar executa a baixa descrita em req sob a chave de idempotência
// informada. Toda a operação — checagem de idempotência, validação de
// saldo de cada item e débito — corre em uma única transação: ou tudo é
// aplicado, ou nada é.
//
// Se chave já tiver sido usada com o mesmo conjunto de itens, devolve o
// resultado salvo anteriormente sem debitar de novo (replay). Se chave já
// tiver sido usada com um conjunto de itens diferente, devolve
// domain.ErrChaveIdempotenciaConflitante.
func (s *BaixaService) Processar(ctx context.Context, chave string, req RequisicaoBaixa) (*RespostaBaixa, error) {
	pool := s.produtoRepo.Pool()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transação de baixa: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	registro, err := s.idempRepo.BuscarPorChaveTx(ctx, tx, chave)
	if err != nil {
		return nil, fmt.Errorf("consultar idempotência: %w", err)
	}

	if registro != nil {
		var respostaSalva RespostaBaixa
		if err := json.Unmarshal(registro.RespostaJSON, &respostaSalva); err != nil {
			return nil, fmt.Errorf("decodificar resposta salva de idempotência: %w", err)
		}

		if !mesmosItens(req.Itens, respostaSalva.Itens) {
			return nil, domain.ErrChaveIdempotenciaConflitante
		}

		// Replay exato: nenhuma alteração de saldo, apenas devolve o que
		// já havia sido processado. Não há necessidade de commit (nada
		// mudou), mas fazê-lo é inofensivo e libera a transação de forma
		// limpa.
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("finalizar transação de replay: %w", err)
		}
		return &respostaSalva, nil
	}

	produtosPorCodigo := make(map[string]*domain.Produto)
	saldosCorrentes := make(map[string]int)
	resultados := make([]ItemBaixaResultado, 0, len(req.Itens))

	for _, item := range req.Itens {
		produto, ja := produtosPorCodigo[item.Codigo]
		if !ja {
			produto, err = s.produtoRepo.BuscarPorCodigoParaAtualizarTx(ctx, tx, item.Codigo)
			if err != nil {
				return nil, err
			}
			produtosPorCodigo[item.Codigo] = produto
			saldosCorrentes[item.Codigo] = produto.Saldo
		}

		saldoAnterior := saldosCorrentes[item.Codigo]
		if saldoAnterior < item.Quantidade {
			return nil, &domain.ErrSaldoInsuficiente{
				Codigo:     item.Codigo,
				Disponivel: saldoAnterior,
				Solicitado: item.Quantidade,
			}
		}

		saldoAtual := saldoAnterior - item.Quantidade
		saldosCorrentes[item.Codigo] = saldoAtual

		resultados = append(resultados, ItemBaixaResultado{
			Codigo:        item.Codigo,
			SaldoAnterior: saldoAnterior,
			SaldoAtual:    saldoAtual,
		})
	}

	for codigo, saldoFinal := range saldosCorrentes {
		produto := produtosPorCodigo[codigo]
		if err := s.produtoRepo.AtualizarSaldoTx(ctx, tx, produto.ID, saldoFinal); err != nil {
			return nil, fmt.Errorf("atualizar saldo de %s: %w", codigo, err)
		}
	}

	resposta := RespostaBaixa{Itens: resultados}
	respostaJSON, err := json.Marshal(resposta)
	if err != nil {
		return nil, fmt.Errorf("codificar resposta de baixa: %w", err)
	}

	if err := s.idempRepo.SalvarTx(ctx, tx, chave, 200, respostaJSON); err != nil {
		return nil, fmt.Errorf("salvar registro de idempotência: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("efetivar transação de baixa: %w", err)
	}

	return &resposta, nil
}

// mesmosItens compara a lista de itens de uma nova requisição com os itens
// reconstruídos a partir de uma resposta de baixa já salva, para decidir se
// uma chave de idempotência repetida representa um replay legítimo (mesmo
// payload) ou um conflito (payload diferente). A quantidade de cada item já
// processado é reconstruída como saldoAnterior - saldoAtual, já que essa
// diferença é, por invariante da baixa atômica, a quantidade debitada.
func mesmosItens(novos []ItemBaixa, salvos []ItemBaixaResultado) bool {
	if len(novos) != len(salvos) {
		return false
	}
	for i, item := range novos {
		quantidadeSalva := salvos[i].SaldoAnterior - salvos[i].SaldoAtual
		if item.Codigo != salvos[i].Codigo || item.Quantidade != quantidadeSalva {
			return false
		}
	}
	return true
}
