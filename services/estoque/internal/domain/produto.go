// Package domain contém as entidades e regras de negócio puras do serviço
// de estoque. Não importa net/http nem qualquer driver de banco de dados.
package domain

import "time"

// Produto representa um item de estoque identificado por um código de
// negócio único. O saldo nunca é negativo — invariante garantida tanto pelo
// banco (CHECK ck_produto_saldo_nao_negativo) quanto pela camada de serviço.
type Produto struct {
	ID           int64
	Codigo       string
	Descricao    string
	Saldo        int
	CriadoEm     time.Time
	AtualizadoEm time.Time
}
