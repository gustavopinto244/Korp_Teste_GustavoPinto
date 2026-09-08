package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/migrations"
)

func abrirPoolDeTeste(t *testing.T) *pgxpool.Pool {
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

func TestProdutoRepository_CRUD(t *testing.T) {
	pool := abrirPoolDeTeste(t)
	repo := repository.NovoProdutoRepository(pool)
	ctx := context.Background()

	// Criar
	produto, err := repo.Criar(ctx, "TESTE-001", "Produto de teste", 10)
	if err != nil {
		t.Fatalf("Criar: %v", err)
	}
	if produto.Codigo != "TESTE-001" || produto.Saldo != 10 {
		t.Fatalf("produto criado inesperado: %+v", produto)
	}

	// Criar com código duplicado
	_, err = repo.Criar(ctx, "TESTE-001", "Outro", 5)
	if err != domain.ErrCodigoDuplicado {
		t.Fatalf("esperava ErrCodigoDuplicado, obteve %v", err)
	}

	// BuscarPorCodigo
	encontrado, err := repo.BuscarPorCodigo(ctx, "TESTE-001")
	if err != nil {
		t.Fatalf("BuscarPorCodigo: %v", err)
	}
	if encontrado.ID != produto.ID {
		t.Fatalf("produto encontrado difere do criado")
	}

	// BuscarPorCodigo inexistente
	_, err = repo.BuscarPorCodigo(ctx, "NAO-EXISTE")
	if err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}

	// Listar
	produtos, err := repo.Listar(ctx)
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(produtos) == 0 {
		t.Fatalf("esperava ao menos um produto na listagem")
	}

	// Atualizar
	atualizado, err := repo.Atualizar(ctx, "TESTE-001", "Descrição nova", 20)
	if err != nil {
		t.Fatalf("Atualizar: %v", err)
	}
	if atualizado.Descricao != "Descrição nova" || atualizado.Saldo != 20 {
		t.Fatalf("produto atualizado inesperado: %+v", atualizado)
	}

	// Atualizar inexistente
	_, err = repo.Atualizar(ctx, "NAO-EXISTE", "x", 1)
	if err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}

	// Remover
	if err := repo.Remover(ctx, "TESTE-001"); err != nil {
		t.Fatalf("Remover: %v", err)
	}
	_, err = repo.BuscarPorCodigo(ctx, "TESTE-001")
	if err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("produto deveria ter sido removido")
	}

	// Remover inexistente
	err = repo.Remover(ctx, "NAO-EXISTE")
	if err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}
}

func TestProdutoRepository_SaldoNaoNegativo(t *testing.T) {
	pool := abrirPoolDeTeste(t)
	repo := repository.NovoProdutoRepository(pool)
	ctx := context.Background()

	if _, err := repo.Criar(ctx, "TESTE-002", "Produto de teste", 5); err != nil {
		t.Fatalf("Criar: %v", err)
	}

	// A constraint CHECK do banco deve impedir saldo negativo mesmo via
	// Atualizar (defesa em profundidade além da validação de serviço).
	_, err := repo.Atualizar(ctx, "TESTE-002", "Produto de teste", -1)
	if err == nil {
		t.Fatalf("esperava erro ao tentar gravar saldo negativo")
	}
}
