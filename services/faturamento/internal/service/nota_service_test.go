package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/testdb"
)

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
	pool := testdb.AbrirPool(t)
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
	pool := testdb.AbrirPool(t)
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
	pool := testdb.AbrirPool(t)
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
