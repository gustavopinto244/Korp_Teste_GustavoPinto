// Package service orquestra os casos de uso do serviço de faturamento,
// coordenando internal/domain, internal/repository e internal/estoqueclient.
package service

import (
	"context"
	"fmt"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
)

// NotaRepositorio é o subconjunto de internal/repository.NotaRepository que
// o NotaService precisa, para facilitar testes com fakes.
type NotaRepositorio interface {
	Criar(ctx context.Context, itens []domain.ItemNota) (domain.NotaFiscal, error)
	Listar(ctx context.Context) ([]domain.NotaFiscal, error)
	BuscarPorID(ctx context.Context, id int64) (domain.NotaFiscal, error)
}

// EstoqueValidador é o subconjunto do cliente de estoque necessário para
// validar itens na criação de uma nota.
type EstoqueValidador interface {
	BuscarProduto(ctx context.Context, codigo string) (estoqueclient.Produto, error)
}

// NotaService implementa os casos de uso de criação/consulta de notas
// fiscais.
type NotaService struct {
	repo    NotaRepositorio
	estoque EstoqueValidador
}

// NovoNotaService constrói um NotaService.
func NovoNotaService(repo NotaRepositorio, estoque EstoqueValidador) *NotaService {
	return &NotaService{repo: repo, estoque: estoque}
}

// Criar valida cada item de entrada contra o catálogo real do estoque
// (capturando a descrição como snapshot) e persiste a nota já com status
// Aberta e numeração vinda da SEQUENCE do banco.
func (s *NotaService) Criar(ctx context.Context, entradas []domain.ItemEntrada) (domain.NotaFiscal, error) {
	if len(entradas) == 0 {
		return domain.NotaFiscal{}, fmt.Errorf("%w: a nota precisa de ao menos um item", domain.ErrProdutoInvalido)
	}

	itens := make([]domain.ItemNota, 0, len(entradas))
	for _, e := range entradas {
		if e.Quantidade <= 0 {
			return domain.NotaFiscal{}, fmt.Errorf("%w: quantidade deve ser positiva para o produto %s", domain.ErrProdutoInvalido, e.ProdutoCodigo)
		}

		produto, err := s.estoque.BuscarProduto(ctx, e.ProdutoCodigo)
		if err != nil {
			return domain.NotaFiscal{}, err
		}

		itens = append(itens, domain.ItemNota{
			ProdutoCodigo:    produto.Codigo,
			ProdutoDescricao: produto.Descricao,
			Quantidade:       e.Quantidade,
		})
	}

	return s.repo.Criar(ctx, itens)
}

// Listar devolve todas as notas fiscais.
func (s *NotaService) Listar(ctx context.Context) ([]domain.NotaFiscal, error) {
	return s.repo.Listar(ctx)
}

// BuscarPorID devolve uma nota com seus itens, ou domain.ErrNotaNaoEncontrada.
func (s *NotaService) BuscarPorID(ctx context.Context, id int64) (domain.NotaFiscal, error) {
	return s.repo.BuscarPorID(ctx, id)
}
