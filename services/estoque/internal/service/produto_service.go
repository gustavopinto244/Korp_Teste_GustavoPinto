// Package service orquestra os casos de uso do serviço de estoque,
// aplicando as regras de negócio sobre as entidades de domain e delegando
// persistência para repository.
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
)

// ProdutoRepositorio é a dependência de persistência exigida pelo
// ProdutoService, extraída como interface para permitir mock em testes de
// domínio sem subir um banco real.
type ProdutoRepositorio interface {
	Criar(ctx context.Context, codigo, descricao string, saldo int) (*domain.Produto, error)
	Listar(ctx context.Context) ([]domain.Produto, error)
	BuscarPorCodigo(ctx context.Context, codigo string) (*domain.Produto, error)
	Atualizar(ctx context.Context, codigo, descricao string, saldo int, saldoEsperado *int) (*domain.Produto, error)
	Desativar(ctx context.Context, codigo string) error
}

// ProdutoService implementa os casos de uso de cadastro de produto.
type ProdutoService struct {
	repo ProdutoRepositorio
}

// NovoProdutoService cria um ProdutoService sobre o repositório informado.
func NovoProdutoService(repo ProdutoRepositorio) *ProdutoService {
	return &ProdutoService{repo: repo}
}

func validarCodigo(codigo string) error {
	if strings.TrimSpace(codigo) == "" {
		return fmt.Errorf("%w: código é obrigatório", domain.ErrValidacao)
	}
	return nil
}

func validarDescricao(descricao string) error {
	if strings.TrimSpace(descricao) == "" {
		return fmt.Errorf("%w: descrição é obrigatória", domain.ErrValidacao)
	}
	return nil
}

func validarSaldo(saldo int) error {
	if saldo < 0 {
		return fmt.Errorf("%w: saldo não pode ser negativo", domain.ErrValidacao)
	}
	return nil
}

// Criar valida e cria um novo produto.
func (s *ProdutoService) Criar(ctx context.Context, codigo, descricao string, saldo int) (*domain.Produto, error) {
	if err := validarCodigo(codigo); err != nil {
		return nil, err
	}
	if err := validarDescricao(descricao); err != nil {
		return nil, err
	}
	if err := validarSaldo(saldo); err != nil {
		return nil, err
	}

	return s.repo.Criar(ctx, codigo, descricao, saldo)
}

// Listar devolve todos os produtos cadastrados.
func (s *ProdutoService) Listar(ctx context.Context) ([]domain.Produto, error) {
	return s.repo.Listar(ctx)
}

// BuscarPorCodigo devolve o detalhe de um produto.
func (s *ProdutoService) BuscarPorCodigo(ctx context.Context, codigo string) (*domain.Produto, error) {
	return s.repo.BuscarPorCodigo(ctx, codigo)
}

// Atualizar valida e atualiza descrição/saldo de um produto existente.
//
// saldoEsperado é opcional: quando informado, a atualização só se aplica se
// o saldo corrente ainda for aquele, o que impede um formulário aberto há
// algum tempo de desfazer o débito de uma impressão. Ver
// ProdutoRepository.Atualizar.
func (s *ProdutoService) Atualizar(ctx context.Context, codigo, descricao string, saldo int, saldoEsperado *int) (*domain.Produto, error) {
	if err := validarDescricao(descricao); err != nil {
		return nil, err
	}
	if err := validarSaldo(saldo); err != nil {
		return nil, err
	}

	return s.repo.Atualizar(ctx, codigo, descricao, saldo, saldoEsperado)
}

// Remover exclui um produto do catálogo. A exclusão é lógica: o produto
// deixa de aparecer em GET /produtos, mas continua existindo para as notas
// fiscais que já o referenciam — inclusive para a baixa de saldo na
// impressão delas. Ver ProdutoRepository.Desativar.
func (s *ProdutoService) Remover(ctx context.Context, codigo string) error {
	return s.repo.Desativar(ctx, codigo)
}
