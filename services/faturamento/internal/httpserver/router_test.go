package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/httpserver"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/httpserver/handler"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/ia"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/testdb"
)

func montarRouterDeTeste(t *testing.T, baseURLEstoque string) (*http.ServeMux, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.AbrirPool(t)

	repo := repository.NovoNotaRepository(pool)
	cliente := estoqueclient.NovoCliente(baseURLEstoque)

	notaService := service.NovoNotaService(repo, cliente)
	imprimirService := service.NovoImprimirService(repo, cliente)

	notaHandler := handler.NovoNotaHandler(notaService)
	imprimirHandler := handler.NovoImprimirHandler(imprimirService)
	interpretarHandler := handler.NovoInterpretarHandler(cliente, ia.NovoInterpretadorMock(), 5*time.Second)

	return httpserver.NovoRouter(pool, notaHandler, imprimirHandler, interpretarHandler), pool
}

func servidorEstoqueFakeCompleto(t *testing.T) *httptest.Server {
	t.Helper()
	catalogo := map[string]string{"PARAF-001": "Parafuso sextavado M6"}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/produtos":
			var produtos []estoqueclient.Produto
			for codigo, descricao := range catalogo {
				produtos = append(produtos, estoqueclient.Produto{Codigo: codigo, Descricao: descricao, Saldo: 10})
			}
			_ = json.NewEncoder(w).Encode(produtos)

		case r.Method == http.MethodGet && len(r.URL.Path) > len("/produtos/"):
			codigo := r.URL.Path[len("/produtos/"):]
			descricao, ok := catalogo[codigo]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"erro": map[string]any{"codigo": "PRODUTO_NAO_ENCONTRADO", "mensagem": "não encontrado", "tipo": "negocio", "repetivel": false},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(estoqueclient.Produto{Codigo: codigo, Descricao: descricao, Saldo: 10})

		case r.Method == http.MethodPost && r.URL.Path == "/produtos/baixa":
			_ = json.NewEncoder(w).Encode(estoqueclient.RespostaBaixa{
				Itens: []estoqueclient.ItemBaixaResultado{{Codigo: "PARAF-001", SaldoAnterior: 10, SaldoAtual: 8}},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRouter_FluxoCompletoCriarEImprimirNota(t *testing.T) {
	servidorEstoque := servidorEstoqueFakeCompleto(t)
	defer servidorEstoque.Close()

	mux, _ := montarRouterDeTeste(t, servidorEstoque.URL)

	// health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	rrHealth := httptest.NewRecorder()
	mux.ServeHTTP(rrHealth, reqHealth)
	if rrHealth.Code != http.StatusOK {
		t.Fatalf("esperava /health 200, obteve %d", rrHealth.Code)
	}

	// criar nota
	corpoCriar := bytes.NewBufferString(`{"itens":[{"produtoCodigo":"PARAF-001","quantidade":2}]}`)
	reqCriar := httptest.NewRequest(http.MethodPost, "/notas", corpoCriar)
	rrCriar := httptest.NewRecorder()
	mux.ServeHTTP(rrCriar, reqCriar)
	if rrCriar.Code != http.StatusCreated {
		t.Fatalf("esperava 201 ao criar nota, obteve %d: %s", rrCriar.Code, rrCriar.Body.String())
	}

	var notaCriada struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rrCriar.Body.Bytes(), &notaCriada); err != nil {
		t.Fatalf("decodificar nota criada: %v", err)
	}
	if notaCriada.Status != "Aberta" {
		t.Fatalf("esperava status Aberta, obteve %s", notaCriada.Status)
	}

	// buscar por id
	reqBuscar := httptest.NewRequest(http.MethodGet, "/notas/"+itoa(notaCriada.ID), nil)
	rrBuscar := httptest.NewRecorder()
	mux.ServeHTTP(rrBuscar, reqBuscar)
	if rrBuscar.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao buscar nota, obteve %d", rrBuscar.Code)
	}

	// listar
	reqListar := httptest.NewRequest(http.MethodGet, "/notas", nil)
	rrListar := httptest.NewRecorder()
	mux.ServeHTTP(rrListar, reqListar)
	if rrListar.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao listar notas, obteve %d", rrListar.Code)
	}

	// imprimir
	reqImprimir := httptest.NewRequest(http.MethodPost, "/notas/"+itoa(notaCriada.ID)+"/imprimir", nil)
	rrImprimir := httptest.NewRecorder()
	mux.ServeHTTP(rrImprimir, reqImprimir)
	if rrImprimir.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao imprimir nota, obteve %d: %s", rrImprimir.Code, rrImprimir.Body.String())
	}

	var notaImpressa struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rrImprimir.Body.Bytes(), &notaImpressa); err != nil {
		t.Fatalf("decodificar nota impressa: %v", err)
	}
	if notaImpressa.Status != "Fechada" {
		t.Fatalf("esperava status Fechada após imprimir, obteve %s", notaImpressa.Status)
	}

	// imprimir de novo: nota já fechada, deve dar 409
	reqImprimir2 := httptest.NewRequest(http.MethodPost, "/notas/"+itoa(notaCriada.ID)+"/imprimir", nil)
	rrImprimir2 := httptest.NewRecorder()
	mux.ServeHTTP(rrImprimir2, reqImprimir2)
	if rrImprimir2.Code != http.StatusConflict {
		t.Fatalf("esperava 409 ao reimprimir nota já fechada, obteve %d", rrImprimir2.Code)
	}
}

func TestRouter_ImprimirNotaInexistente404(t *testing.T) {
	servidorEstoque := servidorEstoqueFakeCompleto(t)
	defer servidorEstoque.Close()

	mux, _ := montarRouterDeTeste(t, servidorEstoque.URL)

	req := httptest.NewRequest(http.MethodPost, "/notas/999999/imprimir", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, obteve %d", rr.Code)
	}
}

func TestRouter_Interpretar(t *testing.T) {
	servidorEstoque := servidorEstoqueFakeCompleto(t)
	defer servidorEstoque.Close()

	mux, _ := montarRouterDeTeste(t, servidorEstoque.URL)

	corpo := bytes.NewBufferString(`{"texto":"2 parafusos sextavados"}`)
	req := httptest.NewRequest(http.MethodPost, "/notas/interpretar", corpo)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rr.Code, rr.Body.String())
	}

	var resultado ia.ResultadoInterpretacao
	if err := json.Unmarshal(rr.Body.Bytes(), &resultado); err != nil {
		t.Fatalf("decodificar resultado: %v", err)
	}
	if len(resultado.ItensSugeridos) != 1 || resultado.ItensSugeridos[0].ProdutoCodigo != "PARAF-001" {
		t.Fatalf("esperava sugerir PARAF-001, obteve %+v", resultado.ItensSugeridos)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
