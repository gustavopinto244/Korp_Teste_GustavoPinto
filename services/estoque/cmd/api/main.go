// Command api é o ponto de entrada do microsserviço de estoque: lê
// configuração via variáveis de ambiente, conecta ao Postgres, aplica
// migrations, monta as camadas repository→service→handler→router e sobe o
// servidor HTTP com graceful shutdown.
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

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/httpserver"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/httpserver/handler"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/migrations"
)

func main() {
	if err := executar(); err != nil {
		log.Fatalf("erro fatal: %v", err)
	}
}

func executar() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("variável de ambiente DATABASE_URL é obrigatória")
	}

	porta := os.Getenv("PORT")
	if porta == "" {
		porta = "8081"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("criar pool de conexões: " + err.Error())
	}
	defer pool.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		return errors.New("conectar ao banco: " + err.Error())
	}

	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		return errors.New("aplicar migrations: " + err.Error())
	}
	log.Println("migrations aplicadas com sucesso")

	produtoRepo := repository.NovoProdutoRepository(pool)
	idempRepo := repository.NovoIdempotenciaRepository()

	produtoSvc := service.NovoProdutoService(produtoRepo)
	baixaSvc := service.NovoBaixaService(produtoRepo, idempRepo)

	produtoHandler := handler.NovoProdutoHandler(produtoSvc)
	baixaHandler := handler.NovoBaixaHandler(baixaSvc)

	mux := httpserver.NovoRouter(produtoHandler, baixaHandler, pool)

	servidor := &http.Server{
		Addr:         ":" + porta,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	erroServidor := make(chan error, 1)
	go func() {
		log.Printf("serviço de estoque escutando na porta %s", porta)
		if err := servidor.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erroServidor <- err
			return
		}
		erroServidor <- nil
	}()

	select {
	case <-ctx.Done():
		log.Println("sinal de encerramento recebido, iniciando graceful shutdown")
	case err := <-erroServidor:
		if err != nil {
			return errors.New("servidor HTTP: " + err.Error())
		}
		return nil
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := servidor.Shutdown(shutdownCtx); err != nil {
		return errors.New("shutdown do servidor: " + err.Error())
	}

	log.Println("servidor encerrado com sucesso")
	return nil
}
