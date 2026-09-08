// Package httpserver monta o roteador HTTP do serviço de faturamento sobre
// net/http.ServeMux (Go 1.22+, roteamento por método e padrões com
// variáveis), sem nenhum framework web.
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/httpserver/handler"
)

// NovoRouter monta o *http.ServeMux com todas as rotas do serviço de
// faturamento.
func NovoRouter(
	pool *pgxpool.Pool,
	notaHandler *handler.NotaHandler,
	imprimirHandler *handler.ImprimirHandler,
	interpretarHandler *handler.InterpretarHandler,
) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(pool))

	mux.HandleFunc("POST /notas", notaHandler.Criar)
	mux.HandleFunc("GET /notas", notaHandler.Listar)
	mux.HandleFunc("GET /notas/{id}", notaHandler.BuscarPorID)
	mux.HandleFunc("POST /notas/{id}/imprimir", imprimirHandler.Imprimir)
	mux.HandleFunc("POST /notas/interpretar", interpretarHandler.Interpretar)

	return mux
}

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "indisponivel"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
