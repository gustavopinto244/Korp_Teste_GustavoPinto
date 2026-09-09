// Package repository implementa o acesso a dados do serviço de estoque
// usando pgx/v5 (pgxpool), sem ORM.
//
// Nenhuma consulta qualifica o schema: as tabelas são resolvidas pelo
// search_path da conexão (DATABASE_URL / TEST_DATABASE_URL_ESTOQUE). É o
// que permite ao mesmo código operar sobre o schema de produção
// ("estoque") e sobre o schema isolado da suíte de testes
// ("estoque_test").
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

// ProdutoRepository encapsula todo o acesso à tabela produto do schema
// apontado pelo search_path da conexão.
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

// Criar insere um novo produto. Um código que pertence a um produto
// desativado é reaproveitado: a linha volta a ficar ativa com a descrição e
// o saldo informados agora. Sem isso, excluir PARAF-001 e tentar cadastrá-lo
// de novo devolveria "código já cadastrado" para um produto que o usuário
// não vê em lugar nenhum. Devolve domain.ErrCodigoDuplicado quando já existe
// um produto ativo com o mesmo código.
func (r *ProdutoRepository) Criar(ctx context.Context, codigo, descricao string, saldo int) (*domain.Produto, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO produto (codigo, descricao, saldo)
		VALUES ($1, $2, $3)
		ON CONFLICT (codigo) DO UPDATE
		   SET descricao = EXCLUDED.descricao,
		       saldo = EXCLUDED.saldo,
		       ativo = true,
		       atualizado_em = now()
		 WHERE produto.ativo = false
		RETURNING `+colunasProduto,
		codigo, descricao, saldo,
	)

	produto, err := escanearProduto(row)
	if err != nil {
		// Sem linha devolvida: o ON CONFLICT casou com um produto ativo e a
		// cláusula WHERE barrou a atualização. escanearProduto traduz
		// pgx.ErrNoRows para ErrProdutoNaoEncontrado, que aqui significa
		// outra coisa.
		if errors.Is(err, domain.ErrProdutoNaoEncontrado) {
			return nil, domain.ErrCodigoDuplicado
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrCodigoDuplicado
		}
		return nil, err
	}
	return produto, nil
}

// Listar devolve o catálogo — apenas produtos ativos, ordenados por código.
// Produtos desativados continuam existindo para as notas que já os
// referenciam, mas não aparecem para quem vai montar uma nota nova.
func (r *ProdutoRepository) Listar(ctx context.Context) ([]domain.Produto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+colunasProduto+`
		FROM produto
		WHERE ativo
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

// BuscarPorCodigo devolve o produto com o código informado, ativo ou não, ou
// domain.ErrProdutoNaoEncontrado se não existir. Produtos desativados
// precisam continuar sendo encontrados: é o que mantém imprimível uma nota
// criada antes da desativação.
func (r *ProdutoRepository) BuscarPorCodigo(ctx context.Context, codigo string) (*domain.Produto, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+colunasProduto+`
		FROM produto
		WHERE codigo = $1`,
		codigo,
	)
	return escanearProduto(row)
}

// Atualizar altera descrição e saldo de um produto existente. Devolve
// domain.ErrProdutoNaoEncontrado se o código não existir.
//
// Este é o outro caminho do sistema que escreve saldo, e o único que escreve
// um valor absoluto vindo da tela. O risco não é a escrita simultânea — o
// Postgres já serializa isso — e sim a leitura velha: a tela carrega saldo
// 100, uma impressão debita 2, e o formulário salva 100 de volta, desfazendo
// o débito de uma nota que já está Fechada.
//
// Por isso o saldo esperado é comparado dentro da transação, com a linha
// travada por SELECT ... FOR UPDATE. Se saldoEsperado for informado e não
// bater com o saldo corrente, a atualização é recusada com
// domain.ErrSaldoDesatualizado e o usuário recarrega antes de decidir.
// saldoEsperado nulo mantém o comportamento de sobrescrever — é o ajuste
// deliberado de inventário, feito por quem sabe o que está fazendo.
func (r *ProdutoRepository) Atualizar(ctx context.Context, codigo, descricao string, saldo int, saldoEsperado *int) (*domain.Produto, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	atual, err := r.BuscarPorCodigoParaAtualizarTx(ctx, tx, codigo)
	if err != nil {
		return nil, err
	}

	if saldoEsperado != nil && *saldoEsperado != atual.Saldo {
		return nil, &domain.ErrSaldoDesatualizado{
			Codigo:   codigo,
			Esperado: *saldoEsperado,
			Atual:    atual.Saldo,
		}
	}

	row := tx.QueryRow(ctx, `
		UPDATE produto
		SET descricao = $2, saldo = $3, atualizado_em = now()
		WHERE codigo = $1
		RETURNING `+colunasProduto,
		codigo, descricao, saldo,
	)

	produto, err := escanearProduto(row)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return produto, nil
}

// Desativar faz a exclusão lógica de um produto: ele sai do catálogo mas
// continua existindo para as notas fiscais que já o referenciam. Devolve
// domain.ErrProdutoNaoEncontrado se o código não existir ou já estiver
// desativado.
//
// A exclusão é lógica porque apagar a linha travava notas: a impressão de
// uma nota Aberta que citasse o produto passava a falhar com
// PRODUTO_NAO_ENCONTRADO, e como o status só vai de Aberta para Fechada, a
// nota ficava sem saída nenhuma.
func (r *ProdutoRepository) Desativar(ctx context.Context, codigo string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE produto
		SET ativo = false, atualizado_em = now()
		WHERE codigo = $1 AND ativo`,
		codigo,
	)
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
		FROM produto
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
		UPDATE produto
		SET saldo = $2, atualizado_em = now()
		WHERE id = $1`,
		id, novoSaldo,
	)
	return err
}
