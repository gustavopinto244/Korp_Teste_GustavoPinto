package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/httpserver"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/httpserver/handler"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/migrations"
)

func subirServidorDeTeste(t *testing.T) *httptest.Server {
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

	produtoRepo := repository.NovoProdutoRepository(pool)
	idempRepo := repository.NovoIdempotenciaRepository()

	produtoSvc := service.NovoProdutoService(produtoRepo)
	baixaSvc := service.NovoBaixaService(produtoRepo, idempRepo)

	produtoHandler := handler.NovoProdutoHandler(produtoSvc)
	baixaHandler := handler.NovoBaixaHandler(baixaSvc)

	mux := httpserver.NovoRouter(produtoHandler, baixaHandler, pool)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func doJSON(t *testing.T, method, url string, corpo any, headers map[string]string) *http.Response {
	t.Helper()

	var body *bytes.Buffer
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body = bytes.NewBuffer(b)
	} else {
		body = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("criar requisição: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("executar requisição: %v", err)
	}
	return resp
}

func TestRouter_CRUDProdutoEHealth(t *testing.T) {
	srv := subirServidorDeTeste(t)

	// GET /health
	resp := doJSON(t, http.MethodGet, srv.URL+"/health", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d, esperado 200", resp.StatusCode)
	}
	resp.Body.Close()

	// POST /produtos
	resp = doJSON(t, http.MethodPost, srv.URL+"/produtos", map[string]any{
		"codigo": "HTTP-001", "descricao": "Produto via HTTP", "saldo": 15,
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /produtos status = %d, esperado 201", resp.StatusCode)
	}
	resp.Body.Close()

	// POST /produtos duplicado -> 409
	resp = doJSON(t, http.MethodPost, srv.URL+"/produtos", map[string]any{
		"codigo": "HTTP-001", "descricao": "Duplicado", "saldo": 1,
	}, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST /produtos duplicado status = %d, esperado 409", resp.StatusCode)
	}
	resp.Body.Close()

	// POST /produtos inválido (saldo negativo) -> 422
	resp = doJSON(t, http.MethodPost, srv.URL+"/produtos", map[string]any{
		"codigo": "HTTP-002", "descricao": "Inválido", "saldo": -1,
	}, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("POST /produtos inválido status = %d, esperado 422", resp.StatusCode)
	}
	resp.Body.Close()

	// GET /produtos
	resp = doJSON(t, http.MethodGet, srv.URL+"/produtos", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /produtos status = %d, esperado 200", resp.StatusCode)
	}
	resp.Body.Close()

	// GET /produtos/{codigo}
	resp = doJSON(t, http.MethodGet, srv.URL+"/produtos/HTTP-001", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /produtos/HTTP-001 status = %d, esperado 200", resp.StatusCode)
	}
	resp.Body.Close()

	// GET /produtos/{codigo} inexistente -> 404
	resp = doJSON(t, http.MethodGet, srv.URL+"/produtos/NAO-EXISTE", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /produtos/NAO-EXISTE status = %d, esperado 404", resp.StatusCode)
	}
	resp.Body.Close()

	// PUT /produtos/{codigo}
	resp = doJSON(t, http.MethodPut, srv.URL+"/produtos/HTTP-001", map[string]any{
		"descricao": "Produto atualizado", "saldo": 30,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /produtos/HTTP-001 status = %d, esperado 200", resp.StatusCode)
	}
	resp.Body.Close()

	// PUT /produtos/{codigo} inexistente -> 404
	resp = doJSON(t, http.MethodPut, srv.URL+"/produtos/NAO-EXISTE", map[string]any{
		"descricao": "x", "saldo": 1,
	}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT /produtos/NAO-EXISTE status = %d, esperado 404", resp.StatusCode)
	}
	resp.Body.Close()

	// DELETE /produtos/{codigo}
	resp = doJSON(t, http.MethodDelete, srv.URL+"/produtos/HTTP-001", nil, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /produtos/HTTP-001 status = %d, esperado 204", resp.StatusCode)
	}
	resp.Body.Close()

	// DELETE /produtos/{codigo} inexistente -> 404
	resp = doJSON(t, http.MethodDelete, srv.URL+"/produtos/HTTP-001", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE /produtos/HTTP-001 (2a vez) status = %d, esperado 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRouter_Baixa(t *testing.T) {
	srv := subirServidorDeTeste(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/produtos", map[string]any{
		"codigo": "BAIXA-001", "descricao": "Produto para baixa", "saldo": 10,
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("criar produto: status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Sem Idempotency-Key -> 400
	resp = doJSON(t, http.MethodPost, srv.URL+"/produtos/baixa", map[string]any{
		"itens": []map[string]any{{"codigo": "BAIXA-001", "quantidade": 2}},
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("baixa sem Idempotency-Key status = %d, esperado 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Com Idempotency-Key -> 200
	resp = doJSON(t, http.MethodPost, srv.URL+"/produtos/baixa", map[string]any{
		"itens": []map[string]any{{"codigo": "BAIXA-001", "quantidade": 2}},
	}, map[string]string{"Idempotency-Key": "teste-http-baixa-1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("baixa status = %d, esperado 200", resp.StatusCode)
	}

	var corpo struct {
		Itens []struct {
			Codigo        string `json:"codigo"`
			SaldoAnterior int    `json:"saldoAnterior"`
			SaldoAtual    int    `json:"saldoAtual"`
		} `json:"itens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	resp.Body.Close()

	if len(corpo.Itens) != 1 || corpo.Itens[0].SaldoAnterior != 10 || corpo.Itens[0].SaldoAtual != 8 {
		t.Fatalf("resposta de baixa inesperada: %+v", corpo)
	}

	// Replay com a mesma chave -> mesmo resultado, sem debitar de novo
	resp = doJSON(t, http.MethodPost, srv.URL+"/produtos/baixa", map[string]any{
		"itens": []map[string]any{{"codigo": "BAIXA-001", "quantidade": 2}},
	}, map[string]string{"Idempotency-Key": "teste-http-baixa-1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replay status = %d, esperado 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/produtos/BAIXA-001", nil, nil)
	var produto struct {
		Saldo int `json:"saldo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&produto); err != nil {
		t.Fatalf("decodificar produto: %v", err)
	}
	resp.Body.Close()
	if produto.Saldo != 8 {
		t.Fatalf("saldo final = %d, esperado 8 (debitado uma única vez)", produto.Saldo)
	}
}
