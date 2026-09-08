package service_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/migrations"
)

func abrirPoolDeTesteBaixa(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL_ESTOQUE")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL_ESTOQUE não definida")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("conectar ao banco: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS estoque CASCADE"); err != nil {
		t.Fatalf("limpar schema: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("limpar schema_migrations: %v", err)
	}

	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}

	return pool
}

func setup(t *testing.T) (*service.BaixaService, *repository.ProdutoRepository) {
	t.Helper()
	pool := abrirPoolDeTesteBaixa(t)
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
