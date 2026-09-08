package service_test

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/service"
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

// servidorEstoqueFake simula o serviço de estoque para os produtos
// informados em produtos (código -> descrição). Qualquer outro código
// devolve 404.
func servidorEstoqueFake(t *testing.T, produtos map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		codigo := r.URL.Path[len("/produtos/"):]
		descricao, ok := produtos[codigo]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"erro": map[string]any{"codigo": "PRODUTO_NAO_ENCONTRADO", "mensagem": "não encontrado", "tipo": "negocio", "repetivel": false},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(estoqueclient.Produto{ID: 1, Codigo: codigo, Descricao: descricao, Saldo: 10})
	}))
}

func TestNotaService_Criar_ValidaItensContraEstoqueECapturaDescricao(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)

	servidor := servidorEstoqueFake(t, map[string]string{"PARAF-001": "Parafuso sextavado M6"})
	defer servidor.Close()

	svc := service.NovoNotaService(repo, estoqueclient.NovoCliente(servidor.URL))

	nota, err := svc.Criar(context.Background(), []domain.ItemEntrada{{ProdutoCodigo: "PARAF-001", Quantidade: 2}})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if nota.Status != domain.StatusAberta {
		t.Fatalf("esperava status Aberta, obteve %s", nota.Status)
	}
	if len(nota.Itens) != 1 || nota.Itens[0].ProdutoDescricao != "Parafuso sextavado M6" {
		t.Fatalf("descrição não capturada como snapshot: %+v", nota.Itens)
	}
}

func TestNotaService_Criar_ProdutoInexistenteRetornaErro(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)

	servidor := servidorEstoqueFake(t, map[string]string{})
	defer servidor.Close()

	svc := service.NovoNotaService(repo, estoqueclient.NovoCliente(servidor.URL))

	_, err := svc.Criar(context.Background(), []domain.ItemEntrada{{ProdutoCodigo: "INEXISTENTE", Quantidade: 1}})
	if err != domain.ErrProdutoInvalido {
		t.Fatalf("esperava ErrProdutoInvalido, obteve %v", err)
	}
}

func TestNotaService_Criar_EstoqueIndisponivelRetorna503(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer servidor.Close()

	svc := service.NovoNotaService(repo, estoqueclient.NovoCliente(servidor.URL))

	_, err := svc.Criar(context.Background(), []domain.ItemEntrada{{ProdutoCodigo: "PARAF-001", Quantidade: 1}})
	if err != domain.ErrEstoqueIndisponivel {
		t.Fatalf("esperava ErrEstoqueIndisponivel, obteve %v", err)
	}
}
