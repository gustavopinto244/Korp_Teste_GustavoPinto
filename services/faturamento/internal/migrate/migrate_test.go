package migrate_test

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
)

//go:embed testdata/*.sql
var testMigrationsFS embed.FS

// versaoDeTeste é o nome do único arquivo em testdata/, usado como chave na
// tabela de controle de migrations.
const versaoDeTeste = "0001_create_schema_e_nota.sql"

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

// schemaDescartavel devolve o schema em que a suíte pode trabalhar: o do
// search_path do DSN de teste. O setup apaga esse schema inteiro a cada
// cenário, então a suíte se recusa a rodar contra um schema que não seja
// declaradamente de teste — é o que impede repetir o acidente de apontar
// TEST_DATABASE_URL_FATURAMENTO para o banco da demo e destruí-lo.
func schemaDescartavel(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	schema, err := migrate.SchemaAlvo(ctx, pool)
	if err != nil {
		t.Fatalf("descobrir schema de teste: %v", err)
	}
	if !strings.HasSuffix(schema, "_test") {
		t.Fatalf("TEST_DATABASE_URL_FATURAMENTO deve apontar para um schema de teste "+
			"(search_path terminando em _test); obtido %q", schema)
	}
	return schema
}

// limparSchemaDeTeste apaga o schema descartável inteiro — inclusive a
// tabela de controle de migrations, que agora mora dentro dele — para que as
// migrations sejam reaplicadas do zero. Nada em public é tocado: aquela
// tabela era compartilhada com o serviço de estoque.
func limparSchemaDeTeste(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	schema := schemaDescartavel(t, ctx, pool)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
		t.Fatalf("limpar schema de teste %s: %v", schema, err)
	}
	return schema
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

func TestAplicar_EIdempotente(t *testing.T) {
	pool := conectarTeste(t)
	ctx := context.Background()
	schema := limparSchemaDeTeste(t, ctx, pool)

	if err := migrate.Aplicar(ctx, pool, testMigrationsFS, "testdata"); err != nil {
		t.Fatalf("primeira aplicação de migrations falhou: %v", err)
	}

	if !tabelaExiste(t, ctx, pool, schema, "nota_fiscal") {
		t.Fatalf("esperava que %s.nota_fiscal existisse após migration", schema)
	}

	// segunda aplicação deve ser um no-op, não deve dar erro (idempotente)
	if err := migrate.Aplicar(ctx, pool, testMigrationsFS, "testdata"); err != nil {
		t.Fatalf("segunda aplicação (idempotente) falhou: %v", err)
	}

	var qtdVersoes int
	consulta := fmt.Sprintf("SELECT count(*) FROM %s.schema_migrations", pgx.Identifier{schema}.Sanitize())
	if err := pool.QueryRow(ctx, consulta).Scan(&qtdVersoes); err != nil {
		t.Fatalf("contar versões aplicadas: %v", err)
	}
	if qtdVersoes != 1 {
		t.Fatalf("esperava 1 versão registrada, obteve %d", qtdVersoes)
	}
}

// TestAplicar_CriaTudoNoSchemaDoSearchPath é a regressão do defeito em que a
// suíte rodava no schema de produção: nada pode ser criado fora do schema do
// search_path, nem mesmo a tabela de controle, que antes vivia em
// public.schema_migrations compartilhada com o serviço de estoque.
func TestAplicar_CriaTudoNoSchemaDoSearchPath(t *testing.T) {
	pool := conectarTeste(t)
	ctx := context.Background()
	schema := limparSchemaDeTeste(t, ctx, pool)

	if err := migrate.Aplicar(ctx, pool, testMigrationsFS, "testdata"); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}

	if !tabelaExiste(t, ctx, pool, schema, "schema_migrations") {
		t.Fatalf("esperava a tabela de controle dentro de %s", schema)
	}
	if tabelaExiste(t, ctx, pool, "public", "nota_fiscal") {
		t.Fatal("a suíte não pode criar tabelas de negócio em public")
	}

	// A tabela de controle antiga é compartilhada com o serviço de estoque:
	// a suíte pode lê-la (adoção de versões legadas), nunca escrever nela.
	var registrosEmPublic int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM public.schema_migrations WHERE versao = $1`,
		versaoDeTeste).Scan(&registrosEmPublic)
	if err == nil && registrosEmPublic != 0 {
		t.Fatal("a suíte não pode gravar em public.schema_migrations, compartilhada com o estoque")
	}
}

// TestAplicar_AdotaControleLegadoEmPublic cobre o caminho de upgrade: um
// banco que já rodou a versão anterior tem as versões registradas em
// public.schema_migrations. O runner precisa adotá-las em vez de tentar
// reaplicar migrations sobre tabelas que já existem.
func TestAplicar_AdotaControleLegadoEmPublic(t *testing.T) {
	pool := conectarTeste(t)
	ctx := context.Background()
	schema := limparSchemaDeTeste(t, ctx, pool)

	// Cria (sem nunca dropar) a tabela antiga e registra a versão como já
	// aplicada, simulando um banco vindo da versão anterior do runner.
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		versao      TEXT PRIMARY KEY,
		aplicado_em TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		t.Fatalf("preparar tabela de controle legada: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"INSERT INTO public.schema_migrations (versao) VALUES ($1) ON CONFLICT DO NOTHING",
		versaoDeTeste); err != nil {
		t.Fatalf("registrar versão legada: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM public.schema_migrations WHERE versao = $1", versaoDeTeste)
	})

	if err := migrate.Aplicar(ctx, pool, testMigrationsFS, "testdata"); err != nil {
		t.Fatalf("aplicar migrations sobre controle legado: %v", err)
	}

	var qtdVersoes int
	consulta := fmt.Sprintf("SELECT count(*) FROM %s.schema_migrations", pgx.Identifier{schema}.Sanitize())
	if err := pool.QueryRow(ctx, consulta).Scan(&qtdVersoes); err != nil {
		t.Fatalf("contar versões adotadas: %v", err)
	}
	if qtdVersoes != 1 {
		t.Fatalf("esperava a versão legada adotada (1), obteve %d", qtdVersoes)
	}

	// A migration foi considerada já aplicada, então não recriou nada — é
	// exatamente o que impede um erro de "relação já existe" num banco que
	// já tem as tabelas.
	if tabelaExiste(t, ctx, pool, schema, "nota_fiscal") {
		t.Fatal("a migration adotada não deveria ter sido reaplicada")
	}
}
