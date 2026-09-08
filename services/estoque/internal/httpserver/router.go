// Package httpserver monta o roteador HTTP do serviço de estoque usando
// net/http + http.ServeMux (Go 1.22+), sem framework web.
package httpserver

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/httpserver/handler"
)

// NovoRouter monta o *http.ServeMux com todas as rotas do serviço.
func NovoRouter(produtoHandler *handler.ProdutoHandler, baixaHandler *handler.BaixaHandler, pool *pgxpool.Pool) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /produtos", produtoHandler.Criar)
	mux.HandleFunc("GET /produtos", produtoHandler.Listar)
	mux.HandleFunc("GET /produtos/{codigo}", produtoHandler.Detalhe)
	mux.HandleFunc("PUT /produtos/{codigo}", produtoHandler.Atualizar)
	mux.HandleFunc("DELETE /produtos/{codigo}", produtoHandler.Remover)

	mux.HandleFunc("POST /produtos/baixa", baixaHandler.Processar)

	mux.HandleFunc("GET /health", healthHandler(pool))

	return mux
}

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"indisponivel"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
