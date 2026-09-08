package migrate_test

import (
	"context"
	"embed"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
)

//go:embed testdata/*.sql
var testMigrationsFS embed.FS

func conectarTeste(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL_FATURAMENTO")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL_FATURAMENTO não configurada, pulando teste de integração")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("conectar ao postgres de teste: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestAplicar_EIdempotente(t *testing.T) {
	pool := conectarTeste(t)
	ctx := context.Background()

	// limpa estado de execuções anteriores
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS public.schema_migrations")
	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS faturamento CASCADE")

	if err := migrate.Aplicar(ctx, pool, testMigrationsFS, "testdata"); err != nil {
		t.Fatalf("primeira aplicação de migrations falhou: %v", err)
	}

	var existe bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'faturamento' AND table_name = 'nota_fiscal'
	)`).Scan(&existe)
	if err != nil {
		t.Fatalf("verificar tabela criada: %v", err)
	}
	if !existe {
		t.Fatal("esperava que faturamento.nota_fiscal existisse após migration")
	}

	// segunda aplicação deve ser um no-op, não deve dar erro (idempotente)
	if err := migrate.Aplicar(ctx, pool, testMigrationsFS, "testdata"); err != nil {
		t.Fatalf("segunda aplicação (idempotente) falhou: %v", err)
	}

	var qtdVersoes int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&qtdVersoes); err != nil {
		t.Fatalf("contar versões aplicadas: %v", err)
	}
	if qtdVersoes != 1 {
		t.Fatalf("esperava 1 versão registrada, obteve %d", qtdVersoes)
	}
}
