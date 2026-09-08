// Package migrate implementa um runner de migrations simples e próprio,
// sem depender de golang-migrate. As migrations são arquivos .sql embutidos
// no binário via embed.FS e aplicadas em ordem alfabética, cada uma dentro
// de uma transação, contra uma tabela de controle schema_migrations.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FS deve ser atribuído pelo chamador com o embed.FS que contém os arquivos
// .sql de migration. Veja cmd/api/main.go para o uso com go:embed.
type FS = embed.FS

// A tabela de controle é sempre qualificada com o schema "public", que
// sempre existe em uma instância Postgres nova, independentemente do
// search_path configurado na conexão (que pode apontar para o schema do
// serviço antes de ele existir, no primeiro boot).
const criarTabelaControle = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    versao      TEXT PRIMARY KEY,
    aplicado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// Aplicar lê todos os arquivos .sql de migrationsFS (no diretório dir),
// ordena alfabeticamente e aplica, dentro de uma transação cada, os que
// ainda não constam em schema_migrations. É seguro chamar mais de uma vez:
// migrations já aplicadas são ignoradas (idempotente).
func Aplicar(ctx context.Context, pool *pgxpool.Pool, migrationsFS fs.FS, dir string) error {
	if _, err := pool.Exec(ctx, criarTabelaControle); err != nil {
		return fmt.Errorf("criar tabela schema_migrations: %w", err)
	}

	entradas, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("ler diretório de migrations %q: %w", dir, err)
	}

	var arquivos []string
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		arquivos = append(arquivos, e.Name())
	}
	sort.Strings(arquivos)

	aplicadas, err := versoesAplicadas(ctx, pool)
	if err != nil {
		return fmt.Errorf("consultar versões aplicadas: %w", err)
	}

	for _, nome := range arquivos {
		if aplicadas[nome] {
			continue
		}

		conteudo, err := fs.ReadFile(migrationsFS, path.Join(dir, nome))
		if err != nil {
			return fmt.Errorf("ler migration %q: %w", nome, err)
		}

		if err := aplicarUma(ctx, pool, nome, string(conteudo)); err != nil {
			return fmt.Errorf("aplicar migration %q: %w", nome, err)
		}
	}

	return nil
}

func versoesAplicadas(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, "SELECT versao FROM public.schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resultado := make(map[string]bool)
	for rows.Next() {
		var versao string
		if err := rows.Scan(&versao); err != nil {
			return nil, err
		}
		resultado[versao] = true
	}
	return resultado, rows.Err()
}

func aplicarUma(ctx context.Context, pool *pgxpool.Pool, nome, sqlConteudo string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, sqlConteudo); err != nil {
		return fmt.Errorf("executar SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO public.schema_migrations (versao) VALUES ($1)", nome); err != nil {
		return fmt.Errorf("registrar versão aplicada: %w", err)
	}

	return tx.Commit(ctx)
}
