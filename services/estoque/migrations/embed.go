// Package migrations expõe os arquivos .sql de migration embutidos no
// binário, para serem aplicados por internal/migrate.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
