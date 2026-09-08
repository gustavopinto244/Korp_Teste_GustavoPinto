// Package migrate implementa um runner de migrations simples e próprio,
// sem depender de golang-migrate. As migrations são arquivos .sql embutidos
// no binário via embed.FS e aplicadas em ordem alfabética, cada uma dentro
// de uma transação, contra uma tabela de controle schema_migrations.
//
// Tudo — schema de negócio, tabela de controle e SQL das migrations — é
// resolvido a partir do search_path da conexão. Nada é hardcodado no nome
// "estoque". É isso que permite que a suíte de testes rode inteiramente
// dentro de um schema próprio (ex.: "estoque_test"), sem tocar nos dados de
// produção nem em objetos compartilhados com o outro microsserviço.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FS deve ser atribuído pelo chamador com o embed.FS que contém os arquivos
// .sql de migration. Veja cmd/api/main.go para o uso com go:embed.
type FS = embed.FS

// EsquemaAlvo devolve o schema em que este serviço opera, derivado do
// search_path da conexão (o primeiro entrada utilizável). É o mesmo schema
// em que as migrations criam as tabelas e em que vive a tabela de controle.
//
// Exportado porque os testes de integração precisam saber qual schema
// limpar — sem isso eles voltariam a hardcodar "estoque" e derrubariam o
// schema de produção.
func EsquemaAlvo(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var searchPath string
	if err := pool.QueryRow(ctx, "SELECT current_setting('search_path')").Scan(&searchPath); err != nil {
		return "", fmt.Errorf("consultar search_path: %w", err)
	}
	return primeiroEsquema(searchPath), nil
}

// primeiroEsquema extrai o primeiro schema utilizável de um search_path
// como `estoque`, `estoque_test, public` ou `"$user", public`. A entrada
// especial "$user" é ignorada por não nomear um schema fixo; se nada
// sobrar, o padrão do Postgres ("public") é usado.
func primeiroEsquema(searchPath string) string {
	for _, parte := range strings.Split(searchPath, ",") {
		nome := strings.Trim(strings.TrimSpace(parte), `"`)
		if nome == "" || nome == "$user" {
			continue
		}
		return nome
	}
	return "public"
}

// Aplicar lê todos os arquivos .sql de migrationsFS (no diretório dir),
// ordena alfabeticamente e aplica, dentro de uma transação cada, os que
// ainda não constam na tabela de controle. É seguro chamar mais de uma vez:
// migrations já aplicadas são ignoradas (idempotente).
func Aplicar(ctx context.Context, pool *pgxpool.Pool, migrationsFS fs.FS, dir string) error {
	esquema, err := EsquemaAlvo(ctx, pool)
	if err != nil {
		return err
	}
	esquemaSQL := pgx.Identifier{esquema}.Sanitize()

	// O schema é criado aqui, e não numa migration, porque seu nome vem do
	// search_path e não pode ser escrito num .sql estático.
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+esquemaSQL); err != nil {
		return fmt.Errorf("criar schema %s: %w", esquema, err)
	}

	// A tabela de controle vive dentro do schema do serviço. Antes ela era
	// public.schema_migrations, compartilhada com o outro microsserviço —
	// o que impedia qualquer isolamento real de teste.
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS `+esquemaSQL+`.schema_migrations (
		    versao      TEXT PRIMARY KEY,
		    aplicado_em TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("criar tabela %s.schema_migrations: %w", esquema, err)
	}

	arquivos, err := listarMigrations(migrationsFS, dir)
	if err != nil {
		return err
	}

	if err := adotarControleLegado(ctx, pool, esquema, esquemaSQL, arquivos); err != nil {
		return fmt.Errorf("adotar controle de migrations legado: %w", err)
	}

	aplicadas, err := versoesAplicadas(ctx, pool, esquemaSQL)
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

		if err := aplicarUma(ctx, pool, esquemaSQL, nome, string(conteudo)); err != nil {
			return fmt.Errorf("aplicar migration %q: %w", nome, err)
		}
	}

	return nil
}

func listarMigrations(migrationsFS fs.FS, dir string) ([]string, error) {
	entradas, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, fmt.Errorf("ler diretório de migrations %q: %w", dir, err)
	}

	var arquivos []string
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		arquivos = append(arquivos, e.Name())
	}
	sort.Strings(arquivos)
	return arquivos, nil
}

// adotarControleLegado cobre a atualização de um banco que já rodou a
// versão anterior deste runner, quando a tabela de controle era
// public.schema_migrations. Nesse banco o schema do serviço já tem as
// tabelas, mas a nova tabela de controle nasceria vazia — e reaplicar a
// primeira migration falharia com "relation ... already exists".
//
// A adoção só acontece quando o schema do serviço realmente já contém
// objetos (prova de que aquelas migrations foram aplicadas nele), e importa
// apenas as versões deste serviço, nunca as do outro — public.schema_migrations
// era compartilhada.
func adotarControleLegado(ctx context.Context, pool *pgxpool.Pool, esquema, esquemaSQL string, arquivos []string) error {
	var jaControlado bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+esquemaSQL+".schema_migrations)").Scan(&jaControlado); err != nil {
		return err
	}
	if jaControlado {
		return nil
	}

	var legadaExiste bool
	if err := pool.QueryRow(ctx,
		"SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&legadaExiste); err != nil {
		return err
	}
	if !legadaExiste {
		return nil
	}

	var objetosNoEsquema int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name <> 'schema_migrations'`,
		esquema,
	).Scan(&objetosNoEsquema); err != nil {
		return err
	}
	if objetosNoEsquema == 0 {
		// Schema vazio: nada foi aplicado nele, então as versões da tabela
		// legada pertencem a outro schema/serviço. Aplicar do zero.
		return nil
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO `+esquemaSQL+`.schema_migrations (versao, aplicado_em)
		SELECT versao, aplicado_em FROM public.schema_migrations WHERE versao = ANY($1)
		ON CONFLICT (versao) DO NOTHING`,
		arquivos,
	)
	return err
}

func versoesAplicadas(ctx context.Context, pool *pgxpool.Pool, esquemaSQL string) (map[string]bool, error) {
	rows, err := pool.Query(ctx, "SELECT versao FROM "+esquemaSQL+".schema_migrations")
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

func aplicarUma(ctx context.Context, pool *pgxpool.Pool, esquemaSQL, nome, sqlConteudo string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// SET LOCAL vale só até o fim desta transação e garante que a SQL sem
	// qualificação de schema crie os objetos no schema alvo, mesmo que a
	// conexão venha sem search_path configurado.
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+esquemaSQL+"; "+sqlConteudo); err != nil {
		return fmt.Errorf("executar SQL: %w", err)
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO "+esquemaSQL+".schema_migrations (versao) VALUES ($1)", nome); err != nil {
		return fmt.Errorf("registrar versão aplicada: %w", err)
	}

	return tx.Commit(ctx)
}
