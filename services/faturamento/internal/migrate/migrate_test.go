package migrate_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/testdb"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/migrations"
)

// nomesDeMigration devolve os arquivos .sql embutidos, para que o teste de
// adoção do controle legado saiba exatamente quais versões são deste
// serviço — e não encoste nas do estoque.
func nomesDeMigration(t *testing.T) []string {
	t.Helper()

	entradas, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("ler migrations embutidas: %v", err)
	}

	var nomes []string
	for _, e := range entradas {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			nomes = append(nomes, e.Name())
		}
	}
	return nomes
}

func TestAplicar_EIdempotente(t *testing.T) {
	pool := testdb.AbrirPool(t) // já aplica as migrations uma vez
	ctx := context.Background()

	// Segunda aplicação deve ser um no-op: nenhuma migration reaplicada e
	// nenhum erro de "relation already exists".
	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("segunda aplicação (idempotente) falhou: %v", err)
	}

	var totalMigrations int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&totalMigrations); err != nil {
		t.Fatalf("consultar schema_migrations: %v", err)
	}
	if esperado := len(nomesDeMigration(t)); totalMigrations != esperado {
		t.Fatalf("esperava %d migrations registradas, obteve %d", esperado, totalMigrations)
	}
}

// TestAplicar_UsaOSchemaDoSearchPath prova o isolamento da suíte: as
// migrations criam objetos no schema do search_path do DSN, e a tabela de
// controle vive dentro dele — não em public.schema_migrations, que é
// compartilhada com o serviço de estoque.
func TestAplicar_UsaOSchemaDoSearchPath(t *testing.T) {
	pool := testdb.AbrirPool(t)
	ctx := context.Background()
	schema := testdb.Schema(t, pool)

	for _, tabela := range []string{"nota_fiscal", "nota_fiscal_item", "schema_migrations"} {
		if !tabelaExiste(t, ctx, pool, schema, tabela) {
			t.Fatalf("tabela %s.%s não foi criada no schema do search_path", schema, tabela)
		}
	}

	if tabelaExiste(t, ctx, pool, "public", "nota_fiscal") {
		t.Fatal("a suíte não pode criar tabelas de negócio em public")
	}
}

// TestAplicar_NaoTocaEmOutrosSchemas prova a consequência mais grave do
// defeito antigo: a suíte derrubava o schema "faturamento" de produção mesmo
// com search_path=faturamento_test. Aqui um schema vizinho com dados é
// criado antes da migração e precisa sobreviver intacto.
func TestAplicar_NaoTocaEmOutrosSchemas(t *testing.T) {
	pool := testdb.AbrirPool(t)
	ctx := context.Background()

	const schemaVizinho = "faturamento_producao_simulada"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schemaVizinho+" CASCADE")
	})

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schemaVizinho+" CASCADE"); err != nil {
		t.Fatalf("preparar schema vizinho: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schemaVizinho); err != nil {
		t.Fatalf("criar schema vizinho: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE "+schemaVizinho+".nota_fiscal (id INT PRIMARY KEY)"); err != nil {
		t.Fatalf("criar tabela no schema vizinho: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO "+schemaVizinho+".nota_fiscal (id) VALUES (1)"); err != nil {
		t.Fatalf("popular schema vizinho: %v", err)
	}

	// Uma rodada completa de setup de teste (drop do schema de teste +
	// migrations) não pode alcançar o schema vizinho.
	testdb.LimparSchema(t, pool)
	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}

	var linhas int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+schemaVizinho+".nota_fiscal").Scan(&linhas); err != nil {
		t.Fatalf("schema vizinho foi destruído pela suíte: %v", err)
	}
	if linhas != 1 {
		t.Fatalf("dados do schema vizinho alterados: %d linhas, esperado 1", linhas)
	}
}

// TestAplicar_AdotaControleLegado cobre a atualização de um banco que já
// rodava a versão anterior do runner, quando a tabela de controle era
// public.schema_migrations. O serviço precisa subir sem tentar reaplicar as
// migrations já aplicadas (que falhariam com "relation already exists").
func TestAplicar_AdotaControleLegado(t *testing.T) {
	pool := testdb.AbrirPool(t)
	ctx := context.Background()

	// public.schema_migrations é compartilhada com o serviço de estoque:
	// este teste anota o estado anterior e devolve exatamente o que
	// encontrou, mexendo apenas nas versões deste serviço.
	arquivos := nomesDeMigration(t)

	var tabelaLegadaExistia bool
	if err := pool.QueryRow(ctx,
		"SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&tabelaLegadaExistia); err != nil {
		t.Fatalf("verificar tabela legada: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if !tabelaLegadaExistia {
			_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS public.schema_migrations")
			return
		}
		_, _ = pool.Exec(ctx, "DELETE FROM public.schema_migrations WHERE versao = ANY($1)", arquivos)
	})

	// Simula o layout legado: tabelas presentes no schema do serviço,
	// nenhuma tabela de controle local, versões registradas na tabela
	// compartilhada public.schema_migrations.
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS public.schema_migrations (
		    versao      TEXT PRIMARY KEY,
		    aplicado_em TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatalf("criar tabela legada: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.schema_migrations (versao)
		SELECT unnest($1::text[]) ON CONFLICT (versao) DO NOTHING`, arquivos); err != nil {
		t.Fatalf("popular tabela legada: %v", err)
	}

	if _, err := pool.Exec(ctx, "DROP TABLE schema_migrations"); err != nil {
		t.Fatalf("remover tabela de controle local: %v", err)
	}

	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("aplicar migrations sobre banco legado: %v", err)
	}

	var totalMigrations int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&totalMigrations); err != nil {
		t.Fatalf("consultar schema_migrations após adoção: %v", err)
	}
	if totalMigrations != len(arquivos) {
		t.Fatalf("esperava %d migrations adotadas, obteve %d", len(arquivos), totalMigrations)
	}
}

func tabelaExiste(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema, tabela string) bool {
	t.Helper()
	var existe bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = $2
	)`, schema, tabela).Scan(&existe)
	if err != nil {
		t.Fatalf("verificar existência de %s.%s: %v", schema, tabela, err)
	}
	return existe
}
