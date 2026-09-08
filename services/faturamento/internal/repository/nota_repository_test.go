package repository_test

import (
	"context"
	"embed"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
)

//go:embed testdata/*.sql
var migrationsFS embed.FS

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
func limparSchemaDeTeste(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	schema := schemaDescartavel(t, ctx, pool)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
		t.Fatalf("limpar schema de teste %s: %v", schema, err)
	}
}

func poolDeTeste(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL_FATURAMENTO")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL_FATURAMENTO não configurada, pulando teste de integração")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("conectar ao postgres de teste: %v", err)
	}
	t.Cleanup(pool.Close)

	limparSchemaDeTeste(t, ctx, pool)
	if err := migrate.Aplicar(ctx, pool, migrationsFS, "testdata"); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}

	return pool
}

func TestCriar_NumeracaoSequencialSemDuplicar(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	nota1, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso", Quantidade: 2}})
	if err != nil {
		t.Fatalf("criar nota 1: %v", err)
	}
	nota2, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "MART-002", ProdutoDescricao: "Martelo", Quantidade: 1}})
	if err != nil {
		t.Fatalf("criar nota 2: %v", err)
	}

	if nota1.Numero == nota2.Numero {
		t.Fatalf("números não deveriam se repetir: %d == %d", nota1.Numero, nota2.Numero)
	}
	if nota2.Numero != nota1.Numero+1 {
		t.Fatalf("esperava numeração sequencial estrita, obteve %d depois de %d", nota2.Numero, nota1.Numero)
	}
	if nota1.Status != domain.StatusAberta {
		t.Fatalf("nota deveria nascer Aberta, veio %s", nota1.Status)
	}
	if len(nota1.Itens) != 1 || nota1.Itens[0].ProdutoCodigo != "PARAF-001" {
		t.Fatalf("itens não persistidos corretamente: %+v", nota1.Itens)
	}
}

func TestBuscarPorID_NaoEncontrada(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	_, err := repo.BuscarPorID(ctx, 99999)
	if err != domain.ErrNotaNaoEncontrada {
		t.Fatalf("esperava ErrNotaNaoEncontrada, obteve %v", err)
	}
}

func TestMarcarFechada_RespeitaWhereStatusAberta(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	nota, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso", Quantidade: 1}})
	if err != nil {
		t.Fatalf("criar nota: %v", err)
	}

	fechada, err := repo.MarcarFechada(ctx, nota.ID)
	if err != nil {
		t.Fatalf("marcar fechada: %v", err)
	}
	if fechada.Status != domain.StatusFechada {
		t.Fatalf("esperava status Fechada, obteve %s", fechada.Status)
	}
	if fechada.FechadoEm == nil {
		t.Fatal("esperava fechado_em preenchido")
	}

	// segunda tentativa: nota já não está Aberta, deve falhar
	_, err = repo.MarcarFechada(ctx, nota.ID)
	if err != domain.ErrNotaNaoAberta {
		t.Fatalf("esperava ErrNotaNaoAberta na segunda tentativa, obteve %v", err)
	}
}

func TestListar_DevolveTodasAsNotas(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	_, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso", Quantidade: 1}})
	if err != nil {
		t.Fatalf("criar nota: %v", err)
	}

	notas, err := repo.Listar(ctx)
	if err != nil {
		t.Fatalf("listar notas: %v", err)
	}
	if len(notas) == 0 {
		t.Fatal("esperava ao menos uma nota listada")
	}
}
