// Package testdb concentra o setup do banco usado pelos testes de
// integração do serviço de estoque. Existe para que a limpeza de schema
// tenha uma implementação única: quando cada arquivo de teste tinha a sua,
// todas hardcodavam o schema "estoque" e derrubavam os dados reais, apesar
// do search_path de teste apontar para outro schema.
//
// Nada em produção importa este pacote.
package testdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/migrations"
)

// SufixoEsquemaDeTeste é exigido no schema apontado por
// TEST_DATABASE_URL_ESTOQUE. É a trava que impede a suíte de apagar o
// schema de produção caso alguém aponte a variável para o banco do
// docker compose.
const SufixoEsquemaDeTeste = "_test"

// AbrirPool conecta ao banco de teste, derruba e recria o schema apontado
// pelo search_path do DSN e aplica todas as migrations nele. O pool é
// fechado automaticamente no fim do teste.
//
// Pula o teste se TEST_DATABASE_URL_ESTOQUE não estiver definida.
func AbrirPool(t *testing.T) *pgxpool.Pool {
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

	LimparEsquema(t, pool)

	if err := migrate.Aplicar(ctx, pool, migrations.FS, "."); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}

	return pool
}

// LimparEsquema derruba o schema de teste inteiro — tabelas de negócio e a
// tabela de controle de migrations, que vive dentro dele. Nada fora desse
// schema é tocado.
func LimparEsquema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	esquema := Esquema(t, pool)

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{esquema}.Sanitize()+" CASCADE"); err != nil {
		t.Fatalf("limpar schema %s: %v", esquema, err)
	}
}

// Esquema devolve o schema de teste derivado do search_path do DSN,
// falhando se ele não parecer um schema de teste.
func Esquema(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	esquema, err := migrate.EsquemaAlvo(context.Background(), pool)
	if err != nil {
		t.Fatalf("descobrir schema alvo: %v", err)
	}

	if !strings.HasSuffix(esquema, SufixoEsquemaDeTeste) {
		t.Fatalf(
			"TEST_DATABASE_URL_ESTOQUE aponta para o schema %q; a suíte derruba o schema inteiro "+
				"e só roda em um schema de teste (search_path terminando em %q)",
			esquema, SufixoEsquemaDeTeste,
		)
	}

	return esquema
}
