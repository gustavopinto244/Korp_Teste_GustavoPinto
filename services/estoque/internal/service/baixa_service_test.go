package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/testdb"
)

func setup(t *testing.T) (*service.BaixaService, *repository.ProdutoRepository) {
	t.Helper()
	pool := testdb.AbrirPool(t)
	produtoRepo := repository.NovoProdutoRepository(pool)
	idempRepo := repository.NovoIdempotenciaRepository()
	return service.NovoBaixaService(produtoRepo, idempRepo), produtoRepo
}

func TestBaixaService_SucessoSimples(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-001", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	resp, err := baixaSvc.Processar(ctx, "chave-1", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "TBX-001", Quantidade: 2}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(resp.Itens) != 1 || resp.Itens[0].SaldoAnterior != 10 || resp.Itens[0].SaldoAtual != 8 {
		t.Fatalf("resposta inesperada: %+v", resp)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-001")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 8 {
		t.Fatalf("saldo no banco = %d, esperado 8", produto.Saldo)
	}
}

func TestBaixaService_SaldoInsuficiente_NenhumItemDebitado(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-001", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto 1: %v", err)
	}
	if _, err := produtoRepo.Criar(ctx, "TBX-002", "Martelo", 1); err != nil {
		t.Fatalf("criar produto 2: %v", err)
	}

	_, err := baixaSvc.Processar(ctx, "chave-2", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{
			{Codigo: "TBX-001", Quantidade: 2}, // teria saldo suficiente
			{Codigo: "TBX-002", Quantidade: 5}, // saldo insuficiente
		},
	})

	var errSaldo *domain.ErrSaldoInsuficiente
	if !errors.As(err, &errSaldo) {
		t.Fatalf("esperava ErrSaldoInsuficiente, obteve %v", err)
	}
	if errSaldo.Codigo != "TBX-002" || errSaldo.Disponivel != 1 || errSaldo.Solicitado != 5 {
		t.Fatalf("erro de saldo insuficiente inesperado: %+v", errSaldo)
	}

	// Nenhum item deve ter sido debitado, nem sequer o que tinha saldo
	// suficiente (tudo ou nada).
	p1, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-001")
	if err != nil {
		t.Fatalf("buscar produto 1: %v", err)
	}
	if p1.Saldo != 10 {
		t.Fatalf("saldo de TBX-001 = %d, esperado 10 (rollback completo)", p1.Saldo)
	}

	p2, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-002")
	if err != nil {
		t.Fatalf("buscar produto 2: %v", err)
	}
	if p2.Saldo != 1 {
		t.Fatalf("saldo de TBX-002 = %d, esperado 1 (rollback completo)", p2.Saldo)
	}
}

// TestBaixaService_QuantidadeNegativaNaoCreditaSaldo reproduz o defeito em
// que uma quantidade negativa passava pela checagem de saldo
// (saldoAnterior < quantidade nunca é verdadeiro para negativos) e o
// débito saldoAnterior - quantidade acabava *creditando* saldo. A baixa é
// dona da invariante de saldo: quantidade não positiva é entrada inválida.
func TestBaixaService_QuantidadeNegativaNaoCreditaSaldo(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-NEG", "Parafuso", 100); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	_, err := baixaSvc.Processar(ctx, "chave-negativa", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "TBX-NEG", Quantidade: -100}},
	})
	if !errors.Is(err, domain.ErrValidacao) {
		t.Fatalf("esperava domain.ErrValidacao para quantidade negativa, obteve %v", err)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-NEG")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 100 {
		t.Fatalf("saldo no banco = %d, esperado 100 (nenhum crédito indevido)", produto.Saldo)
	}
}

func TestBaixaService_QuantidadeZeroRejeitada(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-ZERO", "Parafuso", 7); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	_, err := baixaSvc.Processar(ctx, "chave-zero", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "TBX-ZERO", Quantidade: 0}},
	})
	if !errors.Is(err, domain.ErrValidacao) {
		t.Fatalf("esperava domain.ErrValidacao para quantidade zero, obteve %v", err)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-ZERO")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 7 {
		t.Fatalf("saldo no banco = %d, esperado 7 (nada debitado)", produto.Saldo)
	}
}

func TestBaixaService_ListaDeItensVaziaOuAusente(t *testing.T) {
	baixaSvc, _ := setup(t)
	ctx := context.Background()

	casos := map[string]service.RequisicaoBaixa{
		"lista vazia":   {Itens: []service.ItemBaixa{}},
		"lista ausente": {},
	}

	for nome, req := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := baixaSvc.Processar(ctx, "chave-"+nome, req)
			if !errors.Is(err, domain.ErrValidacao) {
				t.Fatalf("esperava domain.ErrValidacao, obteve %v", err)
			}
		})
	}
}

// Um mesmo código repetido na lista é legítimo (as quantidades se somam),
// mas continua sujeito à validação item a item: uma ocorrência inválida
// invalida a requisição inteira, sem debitar nada.
func TestBaixaService_CodigoRepetidoComQuantidadeInvalida(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-REP", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	_, err := baixaSvc.Processar(ctx, "chave-repetido-invalido", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{
			{Codigo: "TBX-REP", Quantidade: 3},
			{Codigo: "TBX-REP", Quantidade: -5},
		},
	})
	if !errors.Is(err, domain.ErrValidacao) {
		t.Fatalf("esperava domain.ErrValidacao, obteve %v", err)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-REP")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 10 {
		t.Fatalf("saldo no banco = %d, esperado 10 (nada debitado)", produto.Saldo)
	}
}

// Código repetido com quantidades válidas continua funcionando: as
// quantidades se acumulam sobre o mesmo saldo.
func TestBaixaService_CodigoRepetidoComQuantidadesValidas(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-SOMA", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	resp, err := baixaSvc.Processar(ctx, "chave-repetido-valido", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{
			{Codigo: "TBX-SOMA", Quantidade: 3},
			{Codigo: "TBX-SOMA", Quantidade: 2},
		},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(resp.Itens) != 2 || resp.Itens[1].SaldoAtual != 5 {
		t.Fatalf("resposta inesperada: %+v", resp)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-SOMA")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 5 {
		t.Fatalf("saldo no banco = %d, esperado 5", produto.Saldo)
	}
}

func TestBaixaService_ProdutoNaoEncontrado(t *testing.T) {
	baixaSvc, _ := setup(t)
	ctx := context.Background()

	_, err := baixaSvc.Processar(ctx, "chave-3", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "NAO-EXISTE", Quantidade: 1}},
	})
	if !errors.Is(err, domain.ErrProdutoNaoEncontrado) {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}
}

func TestBaixaService_ReplayDeChaveIdempotente(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-001", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	req := service.RequisicaoBaixa{Itens: []service.ItemBaixa{{Codigo: "TBX-001", Quantidade: 2}}}

	resp1, err := baixaSvc.Processar(ctx, "chave-repetida", req)
	if err != nil {
		t.Fatalf("primeira chamada falhou: %v", err)
	}

	resp2, err := baixaSvc.Processar(ctx, "chave-repetida", req)
	if err != nil {
		t.Fatalf("segunda chamada (replay) falhou: %v", err)
	}

	if resp1.Itens[0].SaldoAtual != resp2.Itens[0].SaldoAtual {
		t.Fatalf("replay devolveu resultado diferente: %+v vs %+v", resp1, resp2)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "TBX-001")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	// Debitado uma única vez: 10 - 2 = 8, não 10 - 2 - 2 = 6.
	if produto.Saldo != 8 {
		t.Fatalf("saldo no banco = %d, esperado 8 (debitado apenas uma vez)", produto.Saldo)
	}
}

func TestBaixaService_ChaveRepetidaComPayloadDiferente(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "TBX-001", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	_, err := baixaSvc.Processar(ctx, "chave-conflito", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "TBX-001", Quantidade: 2}},
	})
	if err != nil {
		t.Fatalf("primeira chamada falhou: %v", err)
	}

	_, err = baixaSvc.Processar(ctx, "chave-conflito", service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "TBX-001", Quantidade: 3}},
	})
	if !errors.Is(err, domain.ErrChaveIdempotenciaConflitante) {
		t.Fatalf("esperava ErrChaveIdempotenciaConflitante, obteve %v", err)
	}
}
