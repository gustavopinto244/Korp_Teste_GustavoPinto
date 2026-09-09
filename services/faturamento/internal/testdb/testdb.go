// Package testdb concentra o setup do banco usado pelos testes de
// integração do serviço de faturamento. Existe para que a limpeza de schema
// tenha uma implementação única: quando cada arquivo de teste tinha a sua,
// todas hardcodavam o nome do schema e derrubavam os dados reais, apesar do
// search_path de teste apontar para outro schema.
//
// Nada em produção importa este pacote. Ele é mantido idêntico ao
// internal/testdb/testdb.go do outro microsserviço, exceto pelo nome da
// variável de ambiente e pelos imports do próprio serviço.
package testdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/migrate"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/migrations"
)

// VariavelDSN nomeia a variável de ambiente que aponta para o banco de
// teste deste serviço.
const VariavelDSN = "TEST_DATABASE_URL_FATURAMENTO"

// SufixoSchemaDeTeste é exigido no schema apontado por TEST_DATABASE_URL_FATURAMENTO.
// É a trava que impede a suíte de apagar o schema de produção caso alguém
// aponte a variável para o banco do docker compose.
const SufixoSchemaDeTeste = "_test"

// AbrirPool conecta ao banco de teste, derruba e recria o schema apontado
// pelo search_path do DSN e aplica todas as migrations reais do serviço
// nele. O pool é fechado automaticamente no fim do teste.
//
// Pula o teste se TEST_DATABASE_URL_FATURAMENTO não estiver definida.
func AbrirPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool := Conectar(t)
	LimparSchema(t, pool)

	if err := migrate.Aplicar(context.Background(), pool, migrations.FS, "."); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}

	return pool
}

// Conectar abre o pool contra o banco de teste sem tocar no schema. Serve
// aos testes do próprio runner de migrations, que precisam controlar quando
// as migrations são aplicadas.
func Conectar(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv(VariavelDSN)
	if dsn == "" {
		t.Skipf("%s não configurada, pulando teste de integração", VariavelDSN)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("conectar ao postgres de teste: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// LimparSchema derruba o schema de teste inteiro — tabelas de negócio e a
// tabela de controle de migrations, que vive dentro dele. Nada fora desse
// schema é tocado. Devolve o nome do schema limpo.
func LimparSchema(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	schema := Schema(t, pool)
	if _, err := pool.Exec(context.Background(),
		"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
		t.Fatalf("limpar schema de teste %s: %v", schema, err)
	}
	return schema
}

// Schema devolve o schema de teste derivado do search_path do DSN, falhando
// se ele não parecer um schema de teste.
func Schema(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	schema, err := migrate.SchemaAlvo(context.Background(), pool)
	if err != nil {
		t.Fatalf("descobrir schema alvo: %v", err)
	}

	if !strings.HasSuffix(schema, SufixoSchemaDeTeste) {
		t.Fatalf(
			"%s aponta para o schema %q; a suíte derruba o schema inteiro e só roda em um "+
				"schema de teste (search_path terminando em %q)",
			VariavelDSN, schema, SufixoSchemaDeTeste,
		)
	}

	return schema
}
