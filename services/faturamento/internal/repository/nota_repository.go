// Package repository implementa o acesso a dados do serviço de faturamento
// via pgx/pgxpool, traduzindo o resultado de consultas SQL para as entidades
// de internal/domain.
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

// NotaRepository encapsula todo o acesso a faturamento.nota_fiscal e
// faturamento.nota_fiscal_item.
type NotaRepository struct {
	pool *pgxpool.Pool
}

// NovoNotaRepository constrói um NotaRepository sobre o pool de conexões
// informado.
func NovoNotaRepository(pool *pgxpool.Pool) *NotaRepository {
	return &NotaRepository{pool: pool}
}

// Criar insere uma nova nota (status Aberta, numeração vinda da SEQUENCE do
// banco) junto com seus itens, dentro de uma única transação.
func (r *NotaRepository) Criar(ctx context.Context, itens []domain.ItemNota) (domain.NotaFiscal, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.NotaFiscal{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var nota domain.NotaFiscal
	err = tx.QueryRow(ctx, `
		INSERT INTO faturamento.nota_fiscal DEFAULT VALUES
		RETURNING id, numero, status, criado_em, fechado_em
	`).Scan(&nota.ID, &nota.Numero, &nota.Status, &nota.CriadoEm, &nota.FechadoEm)
	if err != nil {
		return domain.NotaFiscal{}, err
	}

	for i := range itens {
		itens[i].NotaID = nota.ID
		err := tx.QueryRow(ctx, `
			INSERT INTO faturamento.nota_fiscal_item (nota_id, produto_codigo, produto_descricao, quantidade)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, nota.ID, itens[i].ProdutoCodigo, itens[i].ProdutoDescricao, itens[i].Quantidade).Scan(&itens[i].ID)
		if err != nil {
			return domain.NotaFiscal{}, err
		}
	}
	nota.Itens = itens

	if err := tx.Commit(ctx); err != nil {
		return domain.NotaFiscal{}, err
	}

	return nota, nil
}

// Listar devolve todas as notas fiscais, sem itens, ordenadas pela mais
// recente primeiro.
func (r *NotaRepository) Listar(ctx context.Context) ([]domain.NotaFiscal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, numero, status, criado_em, fechado_em
		FROM faturamento.nota_fiscal
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notas []domain.NotaFiscal
	for rows.Next() {
		var n domain.NotaFiscal
		if err := rows.Scan(&n.ID, &n.Numero, &n.Status, &n.CriadoEm, &n.FechadoEm); err != nil {
			return nil, err
		}
		notas = append(notas, n)
	}
	if notas == nil {
		notas = []domain.NotaFiscal{}
	}
	return notas, rows.Err()
}

// BuscarPorID devolve a nota com seus itens carregados, ou
// domain.ErrNotaNaoEncontrada se o ID não existir.
func (r *NotaRepository) BuscarPorID(ctx context.Context, id int64) (domain.NotaFiscal, error) {
	var n domain.NotaFiscal
	err := r.pool.QueryRow(ctx, `
		SELECT id, numero, status, criado_em, fechado_em
		FROM faturamento.nota_fiscal
		WHERE id = $1
	`, id).Scan(&n.ID, &n.Numero, &n.Status, &n.CriadoEm, &n.FechadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotaFiscal{}, domain.ErrNotaNaoEncontrada
		}
		return domain.NotaFiscal{}, err
	}

	itens, err := r.buscarItens(ctx, id)
	if err != nil {
		return domain.NotaFiscal{}, err
	}
	n.Itens = itens

	return n, nil
}

func (r *NotaRepository) buscarItens(ctx context.Context, notaID int64) ([]domain.ItemNota, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nota_id, produto_codigo, produto_descricao, quantidade
		FROM faturamento.nota_fiscal_item
		WHERE nota_id = $1
		ORDER BY id ASC
	`, notaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itens []domain.ItemNota
	for rows.Next() {
		var it domain.ItemNota
		if err := rows.Scan(&it.ID, &it.NotaID, &it.ProdutoCodigo, &it.ProdutoDescricao, &it.Quantidade); err != nil {
			return nil, err
		}
		itens = append(itens, it)
	}
	if itens == nil {
		itens = []domain.ItemNota{}
	}
	return itens, rows.Err()
}

// MarcarFechada marca a nota como Fechada, apenas se ela ainda estiver
// Aberta (segunda trava, além de qualquer checagem prévia em memória, contra
// corrida entre a leitura do status e este UPDATE). Devolve a nota
// atualizada; se RowsAffected != 1 (nota não encontrada ou já não estava
// Aberta), devolve domain.ErrNotaNaoAberta.
func (r *NotaRepository) MarcarFechada(ctx context.Context, id int64) (domain.NotaFiscal, error) {
	ct, err := r.pool.Exec(ctx, `
		UPDATE faturamento.nota_fiscal
		SET status = 'Fechada', fechado_em = now()
		WHERE id = $1 AND status = 'Aberta'
	`, id)
	if err != nil {
		return domain.NotaFiscal{}, err
	}
	if ct.RowsAffected() != 1 {
		return domain.NotaFiscal{}, domain.ErrNotaNaoAberta
	}

	return r.BuscarPorID(ctx, id)
}
