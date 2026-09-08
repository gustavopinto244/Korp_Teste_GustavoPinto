package migrate_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/migrations"
)

func TestAplicar_EIdempotente(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL_ESTOQUE")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL_ESTOQUE não definida")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("conectar ao banco: %v", err)
	}
	defer pool.Close()

	// Limpa estado de execuções anteriores para o teste ser determinístico.
	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS estoque CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS public.schema_migrations")

	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("primeira aplicação falhou: %v", err)
	}

	var totalProdutos int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM estoque.produto").Scan(&totalProdutos); err != nil {
		t.Fatalf("consultar produtos após primeira aplicação: %v", err)
	}
	if totalProdutos == 0 {
		t.Fatalf("esperava produtos de seed após primeira aplicação, obteve 0")
	}

	// Segunda aplicação deve ser um no-op: nenhuma migration reaplicada,
	// nenhum erro, e o seed não deve duplicar linhas.
	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("segunda aplicação (idempotente) falhou: %v", err)
	}

	var totalProdutosDepois int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM estoque.produto").Scan(&totalProdutosDepois); err != nil {
		t.Fatalf("consultar produtos após segunda aplicação: %v", err)
	}
	if totalProdutosDepois != totalProdutos {
		t.Fatalf("segunda aplicação alterou contagem de produtos: antes=%d depois=%d", totalProdutos, totalProdutosDepois)
	}

	var totalMigrations int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&totalMigrations); err != nil {
		t.Fatalf("consultar schema_migrations: %v", err)
	}
	if totalMigrations != 3 {
		t.Fatalf("esperava 3 migrations registradas, obteve %d", totalMigrations)
	}
}
