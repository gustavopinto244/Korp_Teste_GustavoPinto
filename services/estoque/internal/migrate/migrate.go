// Package migrate implementa um runner de migrations simples e próprio, sem
// depender de golang-migrate. As migrations são arquivos .sql embutidos no
// binário via embed.FS e aplicadas em ordem alfabética, cada uma dentro de
// uma transação, contra uma tabela de controle schema_migrations.
//
// Tudo — objetos de negócio, tabela de controle e SQL das migrations — é
// resolvido a partir do search_path da conexão. Nada é qualificado com um
// nome de schema fixo no código, o que permite apontar a suíte de testes
// para um schema descartável (sufixo _test) sem tocar no schema de produção
// nem em objetos compartilhados com o outro microsserviço.
//
// Este arquivo é mantido idêntico ao internal/migrate/migrate.go do outro
// microsserviço. Estoque e faturamento são módulos Go independentes e não
// compartilham código, então a duplicação é deliberada; o que se evita é a
// divergência. Ao alterar um, copie no outro — `diff` entre os dois deve
// continuar vazio.
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

// A tabela de controle mora dentro do schema do próprio serviço (%s é o
// identificador já sanitizado do schema), e não em public: em public ela
// seria compartilhada com o outro microsserviço, que aplica outro conjunto
// de migrations.
const criarTabelaControle = `
CREATE TABLE IF NOT EXISTS %s.schema_migrations (
    versao      TEXT PRIMARY KEY,
    aplicado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// Bancos criados antes de a tabela de controle mudar de lugar têm as versões
// deste serviço registradas em public.schema_migrations. Copiá-las para a
// nova tabela (somente as versões que este serviço conhece, nunca as do
// outro serviço) é o que faz o boot continuar funcionando num banco que já
// tem migrations aplicadas, em vez de tentar reaplicá-las sobre tabelas
// existentes.
const importarControleLegado = `
INSERT INTO %s.schema_migrations (versao, aplicado_em)
SELECT versao, aplicado_em FROM public.schema_migrations
WHERE versao = ANY($1)
ON CONFLICT (versao) DO NOTHING
`

// SchemaAlvo devolve o schema em que este serviço opera, lido do search_path
// da própria conexão. É exportado porque a suíte de testes precisa do mesmo
// nome para limpar o schema descartável antes de cada cenário — sem isso os
// testes voltariam a apagar um schema de nome fixo.
//
// Usa SHOW search_path (e não current_schema()) porque o schema pode ainda
// não existir no primeiro boot, caso em que current_schema() devolveria NULL.
func SchemaAlvo(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var searchPath string
	if err := pool.QueryRow(ctx, "SHOW search_path").Scan(&searchPath); err != nil {
		return "", fmt.Errorf("ler search_path da conexão: %w", err)
	}
	return primeiroSchema(searchPath), nil
}

// primeiroSchema extrai o primeiro schema utilizável de um search_path como
// `estoque`, `estoque_test, public` ou `"$user", public`. A entrada especial
// "$user" é ignorada por não nomear um schema fixo; se nada sobrar, o padrão
// do Postgres ("public") é usado.
func primeiroSchema(searchPath string) string {
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
	schema, err := SchemaAlvo(ctx, pool)
	if err != nil {
		return err
	}
	schemaID := pgx.Identifier{schema}.Sanitize()

	// O schema é criado aqui, e não numa migration, porque seu nome vem do
	// search_path e não pode ser escrito num .sql estático.
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schemaID); err != nil {
		return fmt.Errorf("criar schema %s: %w", schema, err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(criarTabelaControle, schemaID)); err != nil {
		return fmt.Errorf("criar tabela %s.schema_migrations: %w", schema, err)
	}

	arquivos, err := listarMigrations(migrationsFS, dir)
	if err != nil {
		return err
	}

	if err := adotarControleLegado(ctx, pool, schema, schemaID, arquivos); err != nil {
		return fmt.Errorf("adotar controle de migrations legado: %w", err)
	}

	aplicadas, err := versoesAplicadas(ctx, pool, schemaID)
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

		if err := aplicarUma(ctx, pool, schemaID, nome, string(conteudo)); err != nil {
			return fmt.Errorf("aplicar migration %q: %w", nome, err)
		}
	}

	return nil
}

// listarMigrations devolve, em ordem alfabética, os nomes dos arquivos .sql
// do diretório dir de migrationsFS. A ordem alfabética é a ordem de
// aplicação — daí o prefixo numérico no nome de cada migration.
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

// adotarControleLegado cobre a atualização de um banco que já rodou a versão
// anterior deste runner, quando a tabela de controle era
// public.schema_migrations. Nesse banco o schema do serviço já tem as
// tabelas, mas a nova tabela de controle nasceria vazia — e reaplicar a
// primeira migration falharia com "relation ... already exists".
//
// A adoção só acontece quando o schema do serviço realmente já contém
// objetos (prova de que aquelas migrations foram aplicadas nele), e importa
// apenas as versões deste serviço, nunca as do outro — public.schema_migrations
// era compartilhada. A tabela legada nunca é apagada: ela pode ainda estar em
// uso pelo outro serviço.
func adotarControleLegado(ctx context.Context, pool *pgxpool.Pool, schema, schemaID string, arquivos []string) error {
	if len(arquivos) == 0 {
		return nil
	}

	var jaControlado bool
	if err := pool.QueryRow(ctx,
		fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s.schema_migrations)", schemaID)).Scan(&jaControlado); err != nil {
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

	var objetosNoSchema int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name <> 'schema_migrations'`,
		schema,
	).Scan(&objetosNoSchema); err != nil {
		return err
	}
	if objetosNoSchema == 0 {
		// Schema vazio: nada foi aplicado nele, então as versões da tabela
		// legada pertencem a outro schema/serviço. Aplicar do zero.
		return nil
	}

	_, err := pool.Exec(ctx, fmt.Sprintf(importarControleLegado, schemaID), arquivos)
	return err
}

// versoesAplicadas devolve o conjunto de migrations já registradas na tabela
// de controle do schema.
func versoesAplicadas(ctx context.Context, pool *pgxpool.Pool, schemaID string) (map[string]bool, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT versao FROM %s.schema_migrations", schemaID))
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

// aplicarUma executa o SQL de uma migration e registra sua versão na tabela
// de controle dentro da mesma transação: ou as duas coisas acontecem, ou
// nenhuma.
func aplicarUma(ctx context.Context, pool *pgxpool.Pool, schemaID, nome, sqlConteudo string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// As migrations não qualificam nomes de objeto: quem decide onde elas
	// criam as tabelas é este search_path, restrito à transação.
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+schemaID); err != nil {
		return fmt.Errorf("posicionar search_path em %s: %w", schemaID, err)
	}

	if _, err := tx.Exec(ctx, sqlConteudo); err != nil {
		return fmt.Errorf("executar SQL: %w", err)
	}

	if _, err := tx.Exec(ctx,
		fmt.Sprintf("INSERT INTO %s.schema_migrations (versao) VALUES ($1)", schemaID), nome); err != nil {
		return fmt.Errorf("registrar versão aplicada: %w", err)
	}

	return tx.Commit(ctx)
}
