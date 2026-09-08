package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// RegistroIdempotencia é o resultado salvo de uma baixa já processada,
// usado para replay exato quando a mesma chave de idempotência repetir.
type RegistroIdempotencia struct {
	Chave        string
	StatusHTTP   int
	RespostaJSON []byte
}

// IdempotenciaRepository encapsula o acesso à tabela idempotencia_baixa do
// schema apontado pelo search_path da conexão.
type IdempotenciaRepository struct{}

// NovoIdempotenciaRepository cria um IdempotenciaRepository. Não guarda
// pool próprio: todos os métodos recebem o executor (pool ou transação)
// explicitamente, pois a baixa precisa que essas consultas participem da
// mesma transação que valida e debita o saldo.
func NovoIdempotenciaRepository() *IdempotenciaRepository {
	return &IdempotenciaRepository{}
}

// BuscarPorChaveTx busca um registro de idempotência já salvo. Devolve
// (nil, nil) se a chave ainda não existir.
func (r *IdempotenciaRepository) BuscarPorChaveTx(ctx context.Context, tx DBTX, chave string) (*RegistroIdempotencia, error) {
	row := tx.QueryRow(ctx, `
		SELECT chave, status_http, resposta_json
		FROM idempotencia_baixa
		WHERE chave = $1`,
		chave,
	)

	var reg RegistroIdempotencia
	if err := row.Scan(&reg.Chave, &reg.StatusHTTP, &reg.RespostaJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &reg, nil
}

// SalvarTx grava o resultado de uma baixa recém-processada, para permitir
// replay em chamadas futuras com a mesma chave.
func (r *IdempotenciaRepository) SalvarTx(ctx context.Context, tx DBTX, chave string, statusHTTP int, respostaJSON []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO idempotencia_baixa (chave, status_http, resposta_json)
		VALUES ($1, $2, $3)`,
		chave, statusHTTP, respostaJSON,
	)
	return err
}
