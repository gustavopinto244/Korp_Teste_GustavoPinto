// Package migrations embute os arquivos .sql deste diretório no binário via
// go:embed, para que cmd/api/main.go possa aplicá-los em tempo de boot sem
// depender do sistema de arquivos do container em runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
