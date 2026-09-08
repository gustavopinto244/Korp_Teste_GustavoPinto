// Package repository implementa o acesso a dados do serviço de estoque
// usando pgx/v5 (pgxpool), sem ORM.
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
)

// DBTX é a interface mínima comum a *pgxpool.Pool e pgx.Tx, o que permite
// que os métodos de repositório sejam usados tanto fora quanto dentro de
// uma transação (necessário para a baixa atômica de múltiplos produtos).
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ProdutoRepository encapsula todo o acesso à tabela estoque.produto.
type ProdutoRepository struct {
	pool *pgxpool.Pool
}

// NovoProdutoRepository cria um ProdutoRepository sobre o pool informado.
func NovoProdutoRepository(pool *pgxpool.Pool) *ProdutoRepository {
	return &ProdutoRepository{pool: pool}
}

// Pool devolve o pool de conexões subjacente, usado pelo serviço de baixa
// para abrir a transação que engloba a verificação de idempotência e o
// débito de saldo de múltiplos produtos.
func (r *ProdutoRepository) Pool() *pgxpool.Pool {
	return r.pool
}

const colunasProduto = "id, codigo, descricao, saldo, criado_em, atualizado_em"

func escanearProduto(row pgx.Row) (*domain.Produto, error) {
	var p domain.Produto
	if err := row.Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Saldo, &p.CriadoEm, &p.AtualizadoEm); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProdutoNaoEncontrado
		}
		return nil, err
	}
	return &p, nil
}

// Criar insere um novo produto. Devolve domain.ErrCodigoDuplicado se já
// existir um produto com o mesmo código.
func (r *ProdutoRepository) Criar(ctx context.Context, codigo, descricao string, saldo int) (*domain.Produto, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO estoque.produto (codigo, descricao, saldo)
		VALUES ($1, $2, $3)
		RETURNING `+colunasProduto,
		codigo, descricao, saldo,
	)

	produto, err := escanearProduto(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrCodigoDuplicado
		}
		return nil, err
	}
	return produto, nil
}

// Listar devolve todos os produtos cadastrados, ordenados por código.
func (r *ProdutoRepository) Listar(ctx context.Context) ([]domain.Produto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+colunasProduto+`
		FROM estoque.produto
		ORDER BY codigo`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var produtos []domain.Produto
	for rows.Next() {
		var p domain.Produto
		if err := rows.Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Saldo, &p.CriadoEm, &p.AtualizadoEm); err != nil {
			return nil, err
		}
		produtos = append(produtos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if produtos == nil {
		produtos = []domain.Produto{}
	}
	return produtos, nil
}

// BuscarPorCodigo devolve o produto com o código informado, ou
// domain.ErrProdutoNaoEncontrado se não existir.
func (r *ProdutoRepository) BuscarPorCodigo(ctx context.Context, codigo string) (*domain.Produto, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+colunasProduto+`
		FROM estoque.produto
		WHERE codigo = $1`,
		codigo,
	)
	return escanearProduto(row)
}

// Atualizar altera descrição e saldo de um produto existente. Devolve
// domain.ErrProdutoNaoEncontrado se o código não existir.
func (r *ProdutoRepository) Atualizar(ctx context.Context, codigo, descricao string, saldo int) (*domain.Produto, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE estoque.produto
		SET descricao = $2, saldo = $3, atualizado_em = now()
		WHERE codigo = $1
		RETURNING `+colunasProduto,
		codigo, descricao, saldo,
	)
	return escanearProduto(row)
}

// Remover exclui um produto pelo código. Devolve
// domain.ErrProdutoNaoEncontrado se o código não existir.
func (r *ProdutoRepository) Remover(ctx context.Context, codigo string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM estoque.produto WHERE codigo = $1`, codigo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProdutoNaoEncontrado
	}
	return nil
}

// BuscarPorCodigoParaAtualizarTx busca um produto por código dentro de uma
// transação, com SELECT ... FOR UPDATE, garantindo que a leitura do saldo
// usada para validar e debitar não sofra alteração concorrente até o
// commit. Devolve domain.ErrProdutoNaoEncontrado se não existir.
func (r *ProdutoRepository) BuscarPorCodigoParaAtualizarTx(ctx context.Context, tx DBTX, codigo string) (*domain.Produto, error) {
	row := tx.QueryRow(ctx, `
		SELECT `+colunasProduto+`
		FROM estoque.produto
		WHERE codigo = $1
		FOR UPDATE`,
		codigo,
	)
	return escanearProduto(row)
}

// AtualizarSaldoTx debita/ajusta o saldo de um produto (por id) dentro de
// uma transação.
func (r *ProdutoRepository) AtualizarSaldoTx(ctx context.Context, tx DBTX, id int64, novoSaldo int) error {
	_, err := tx.Exec(ctx, `
		UPDATE estoque.produto
		SET saldo = $2, atualizado_em = now()
		WHERE id = $1`,
		id, novoSaldo,
	)
	return err
}
