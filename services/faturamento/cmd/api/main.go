// Command api é o ponto de entrada do serviço de faturamento: lê
// configuração de variáveis de ambiente, conecta ao Postgres, aplica
// migrations, monta o roteador HTTP e sobe o servidor com graceful
// shutdown.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/httpserver"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/httpserver/handler"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/ia"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/migrations"
)

func main() {
	if err := executar(); err != nil {
		log.Fatalf("faturamento: erro fatal: %v", err)
	}
}

func executar() error {
	databaseURL := obrigatorio("DATABASE_URL")
	porta := comPadrao("PORT", "8082")
	estoqueBaseURL := obrigatorio("ESTOQUE_BASE_URL")
	iaProvider := comPadrao("IA_PROVIDER", "mock")
	iaAPIKey := os.Getenv("IA_API_KEY")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctxMigracao, cancelMigracao := context.WithTimeout(ctx, 30*time.Second)
	defer cancelMigracao()
	if err := migrate.Aplicar(ctxMigracao, pool, migrations.FS, "."); err != nil {
		return err
	}

	cliente := estoqueclient.NovoCliente(estoqueBaseURL)
	notaRepo := repository.NovoNotaRepository(pool)

	notaService := service.NovoNotaService(notaRepo, cliente)
	imprimirService := service.NovoImprimirService(notaRepo, cliente)
	interpretador := ia.NovoInterpretador(iaProvider, iaAPIKey)

	notaHandler := handler.NovoNotaHandler(notaService)
	imprimirHandler := handler.NovoImprimirHandler(imprimirService)
	interpretarHandler := handler.NovoInterpretarHandler(cliente, interpretador)

	mux := httpserver.NovoRouter(pool, notaHandler, imprimirHandler, interpretarHandler)

	servidor := &http.Server{
		Addr:    ":" + porta,
		Handler: mux,
		// maior que o tempo total do cliente de estoque (~2.5s com
		// retries), para nunca cortar a resposta no meio de uma tentativa
		// de impressão.
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  10 * time.Second,
	}

	erroServidor := make(chan error, 1)
	go func() {
		log.Printf("faturamento: ouvindo na porta %s", porta)
		if err := servidor.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erroServidor <- err
			return
		}
		erroServidor <- nil
	}()

	sinal := make(chan os.Signal, 1)
	signal.Notify(sinal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-erroServidor:
		return err
	case <-sinal:
		log.Println("faturamento: encerrando graciosamente...")
	}

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	return servidor.Shutdown(ctxShutdown)
}

func obrigatorio(chave string) string {
	valor := os.Getenv(chave)
	if valor == "" {
		log.Fatalf("faturamento: variável de ambiente obrigatória %s não configurada", chave)
	}
	return valor
}

func comPadrao(chave, padrao string) string {
	if valor := os.Getenv(chave); valor != "" {
		return valor
	}
	return padrao
}
