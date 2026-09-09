package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

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

// validarRequisicaoBaixa rejeita entradas que violariam a invariante de
// saldo antes de qualquer acesso ao banco. A baixa é a dona dessa
// invariante: sem esta validação, uma quantidade negativa passa ileso pela
// checagem `saldoAnterior < item.Quantidade` (nunca verdadeira para
// negativos) e o débito `saldoAnterior - item.Quantidade` acaba
// *creditando* saldo — algo que o CHECK (saldo >= 0) do banco não barra,
// pois o valor sobe.
//
// Um mesmo código repetido na lista continua sendo aceito (as quantidades
// se somam sobre o saldo corrente), mas cada ocorrência é validada
// individualmente: basta uma inválida para a requisição inteira ser
// rejeitada, sem debitar nada.
func validarRequisicaoBaixa(req RequisicaoBaixa) error {
	if len(req.Itens) == 0 {
		return fmt.Errorf("%w: a baixa exige ao menos um item", domain.ErrValidacao)
	}

	for _, item := range req.Itens {
		if strings.TrimSpace(item.Codigo) == "" {
			return fmt.Errorf("%w: código do produto é obrigatório em todos os itens", domain.ErrValidacao)
		}
		if item.Quantidade <= 0 {
			return fmt.Errorf(
				"%w: quantidade do produto %s deve ser maior que zero (recebido: %d)",
				domain.ErrValidacao, item.Codigo, item.Quantidade,
			)
		}
	}

	return nil
}

// Processar executa a baixa descrita em req sob a chave de idempotência
// informada. Toda a operação — checagem de idempotência, validação de
// saldo de cada item e débito — corre em uma única transação: ou tudo é
// aplicado, ou nada é.
//
// A requisição é validada antes de a transação abrir: lista de itens vazia
// ou quantidade não positiva devolvem domain.ErrValidacao (422) sem tocar
// no banco.
//
// Se chave já tiver sido usada com o mesmo conjunto de itens, devolve o
// resultado salvo anteriormente sem debitar de novo (replay). Se chave já
// tiver sido usada com um conjunto de itens diferente, devolve
// domain.ErrChaveIdempotenciaConflitante.
//
// Duas requisições idênticas disparadas ao mesmo tempo (o duplo clique em
// Imprimir) podem passar as duas pela checagem de idempotência antes de
// qualquer commit. Nesse caso a perdedora recebe
// repository.ErrChaveJaRegistrada ao gravar a chave e é reprocessada uma
// vez: a vencedora já commitou, então a segunda passada encontra o registro
// e devolve o mesmo replay que uma chamada sequencial devolveria. Sem isso a
// perdedora respondia erro de banco (violação de unicidade) para o usuário.
func (s *BaixaService) Processar(ctx context.Context, chave string, req RequisicaoBaixa) (*RespostaBaixa, error) {
	if err := validarRequisicaoBaixa(req); err != nil {
		return nil, err
	}

	resposta, err := s.processarUmaVez(ctx, chave, req)
	if errors.Is(err, repository.ErrChaveJaRegistrada) {
		return s.processarUmaVez(ctx, chave, req)
	}
	return resposta, err
}

// processarUmaVez é uma tentativa única da baixa, em uma transação.
func (s *BaixaService) processarUmaVez(ctx context.Context, chave string, req RequisicaoBaixa) (*RespostaBaixa, error) {
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

	// As linhas são travadas em ordem de código, e não na ordem em que o
	// cliente mandou os itens. Duas notas com os mesmos produtos em ordem
	// oposta travariam uma a linha que a outra espera — deadlock que o
	// Postgres resolve abortando uma das transações. Ordem total única
	// elimina o ciclo: as duas disputam as mesmas linhas na mesma sequência
	// e uma apenas espera a outra.
	produtosPorCodigo := make(map[string]*domain.Produto)
	saldosCorrentes := make(map[string]int)

	for _, codigo := range codigosOrdenados(req.Itens) {
		produto, err := s.produtoRepo.BuscarPorCodigoParaAtualizarTx(ctx, tx, codigo)
		if err != nil {
			return nil, err
		}
		produtosPorCodigo[codigo] = produto
		saldosCorrentes[codigo] = produto.Saldo
	}

	resultados := make([]ItemBaixaResultado, 0, len(req.Itens))

	for _, item := range req.Itens {
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

// codigosOrdenados devolve os códigos distintos dos itens em ordem
// alfabética — a ordem em que as linhas de produto são travadas.
func codigosOrdenados(itens []ItemBaixa) []string {
	vistos := make(map[string]bool, len(itens))
	codigos := make([]string, 0, len(itens))
	for _, item := range itens {
		if vistos[item.Codigo] {
			continue
		}
		vistos[item.Codigo] = true
		codigos = append(codigos, item.Codigo)
	}
	sort.Strings(codigos)
	return codigos
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
