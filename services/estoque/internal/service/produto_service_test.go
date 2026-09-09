package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
)

// produtoRepositorioMock é uma implementação em memória de
// service.ProdutoRepositorio, usada para testar as regras de negócio de
// ProdutoService sem depender de um banco real.
type produtoRepositorioMock struct {
	produtos map[string]*domain.Produto
}

func novoMock() *produtoRepositorioMock {
	return &produtoRepositorioMock{produtos: make(map[string]*domain.Produto)}
}

func (m *produtoRepositorioMock) Criar(_ context.Context, codigo, descricao string, saldo int) (*domain.Produto, error) {
	if _, existe := m.produtos[codigo]; existe {
		return nil, domain.ErrCodigoDuplicado
	}
	p := &domain.Produto{ID: int64(len(m.produtos) + 1), Codigo: codigo, Descricao: descricao, Saldo: saldo}
	m.produtos[codigo] = p
	return p, nil
}

func (m *produtoRepositorioMock) Listar(_ context.Context) ([]domain.Produto, error) {
	var lista []domain.Produto
	for _, p := range m.produtos {
		lista = append(lista, *p)
	}
	return lista, nil
}

func (m *produtoRepositorioMock) BuscarPorCodigo(_ context.Context, codigo string) (*domain.Produto, error) {
	p, existe := m.produtos[codigo]
	if !existe {
		return nil, domain.ErrProdutoNaoEncontrado
	}
	return p, nil
}

func (m *produtoRepositorioMock) Atualizar(_ context.Context, codigo, descricao string, saldo int, saldoEsperado *int) (*domain.Produto, error) {
	p, existe := m.produtos[codigo]
	if !existe {
		return nil, domain.ErrProdutoNaoEncontrado
	}
	p.Descricao = descricao
	p.Saldo = saldo
	return p, nil
}

func (m *produtoRepositorioMock) Desativar(_ context.Context, codigo string) error {
	if _, existe := m.produtos[codigo]; !existe {
		return domain.ErrProdutoNaoEncontrado
	}
	delete(m.produtos, codigo)
	return nil
}

func TestProdutoService_Criar(t *testing.T) {
	ctx := context.Background()

	t.Run("sucesso", func(t *testing.T) {
		s := service.NovoProdutoService(novoMock())
		p, err := s.Criar(ctx, "PARAF-001", "Parafuso", 10)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if p.Codigo != "PARAF-001" || p.Saldo != 10 {
			t.Fatalf("produto inesperado: %+v", p)
		}
	})

	t.Run("codigo vazio", func(t *testing.T) {
		s := service.NovoProdutoService(novoMock())
		_, err := s.Criar(ctx, "", "Parafuso", 10)
		if !errors.Is(err, domain.ErrValidacao) {
			t.Fatalf("esperava ErrValidacao, obteve %v", err)
		}
	})

	t.Run("descricao vazia", func(t *testing.T) {
		s := service.NovoProdutoService(novoMock())
		_, err := s.Criar(ctx, "PARAF-001", "  ", 10)
		if !errors.Is(err, domain.ErrValidacao) {
			t.Fatalf("esperava ErrValidacao, obteve %v", err)
		}
	})

	t.Run("saldo negativo", func(t *testing.T) {
		s := service.NovoProdutoService(novoMock())
		_, err := s.Criar(ctx, "PARAF-001", "Parafuso", -1)
		if !errors.Is(err, domain.ErrValidacao) {
			t.Fatalf("esperava ErrValidacao, obteve %v", err)
		}
	})

	t.Run("codigo duplicado", func(t *testing.T) {
		mock := novoMock()
		s := service.NovoProdutoService(mock)
		if _, err := s.Criar(ctx, "PARAF-001", "Parafuso", 10); err != nil {
			t.Fatalf("primeira criação falhou: %v", err)
		}
		_, err := s.Criar(ctx, "PARAF-001", "Outro parafuso", 5)
		if !errors.Is(err, domain.ErrCodigoDuplicado) {
			t.Fatalf("esperava ErrCodigoDuplicado, obteve %v", err)
		}
	})
}

func TestProdutoService_Atualizar(t *testing.T) {
	ctx := context.Background()

	t.Run("sucesso", func(t *testing.T) {
		mock := novoMock()
		s := service.NovoProdutoService(mock)
		if _, err := s.Criar(ctx, "PARAF-001", "Parafuso", 10); err != nil {
			t.Fatalf("criar: %v", err)
		}
		p, err := s.Atualizar(ctx, "PARAF-001", "Parafuso novo", 20, nil)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if p.Descricao != "Parafuso novo" || p.Saldo != 20 {
			t.Fatalf("produto inesperado: %+v", p)
		}
	})

	t.Run("nao encontrado", func(t *testing.T) {
		s := service.NovoProdutoService(novoMock())
		_, err := s.Atualizar(ctx, "NAO-EXISTE", "x", 1, nil)
		if !errors.Is(err, domain.ErrProdutoNaoEncontrado) {
			t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
		}
	})

	t.Run("saldo negativo", func(t *testing.T) {
		mock := novoMock()
		s := service.NovoProdutoService(mock)
		if _, err := s.Criar(ctx, "PARAF-001", "Parafuso", 10); err != nil {
			t.Fatalf("criar: %v", err)
		}
		_, err := s.Atualizar(ctx, "PARAF-001", "Parafuso", -5, nil)
		if !errors.Is(err, domain.ErrValidacao) {
			t.Fatalf("esperava ErrValidacao, obteve %v", err)
		}
	})
}

func TestProdutoService_Remover(t *testing.T) {
	ctx := context.Background()

	t.Run("nao encontrado", func(t *testing.T) {
		s := service.NovoProdutoService(novoMock())
		err := s.Remover(ctx, "NAO-EXISTE")
		if !errors.Is(err, domain.ErrProdutoNaoEncontrado) {
			t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
		}
	})
}
