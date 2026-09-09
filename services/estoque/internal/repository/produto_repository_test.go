package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/testdb"
)

func TestProdutoRepository_CRUD(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoProdutoRepository(pool)
	ctx := context.Background()

	// Criar
	produto, err := repo.Criar(ctx, "TESTE-001", "Produto de teste", 10)
	if err != nil {
		t.Fatalf("Criar: %v", err)
	}
	if produto.Codigo != "TESTE-001" || produto.Saldo != 10 {
		t.Fatalf("produto criado inesperado: %+v", produto)
	}

	// Criar com código duplicado
	_, err = repo.Criar(ctx, "TESTE-001", "Outro", 5)
	if err != domain.ErrCodigoDuplicado {
		t.Fatalf("esperava ErrCodigoDuplicado, obteve %v", err)
	}

	// BuscarPorCodigo
	encontrado, err := repo.BuscarPorCodigo(ctx, "TESTE-001")
	if err != nil {
		t.Fatalf("BuscarPorCodigo: %v", err)
	}
	if encontrado.ID != produto.ID {
		t.Fatalf("produto encontrado difere do criado")
	}

	// BuscarPorCodigo inexistente
	_, err = repo.BuscarPorCodigo(ctx, "NAO-EXISTE")
	if err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}

	// Listar
	produtos, err := repo.Listar(ctx)
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(produtos) == 0 {
		t.Fatalf("esperava ao menos um produto na listagem")
	}

	// Atualizar
	atualizado, err := repo.Atualizar(ctx, "TESTE-001", "Descrição nova", 20, nil)
	if err != nil {
		t.Fatalf("Atualizar: %v", err)
	}
	if atualizado.Descricao != "Descrição nova" || atualizado.Saldo != 20 {
		t.Fatalf("produto atualizado inesperado: %+v", atualizado)
	}

	// Atualizar inexistente
	_, err = repo.Atualizar(ctx, "NAO-EXISTE", "x", 1, nil)
	if err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}

	// Desativar tira o produto do catálogo...
	if err := repo.Desativar(ctx, "TESTE-001"); err != nil {
		t.Fatalf("Desativar: %v", err)
	}
	catalogo, err := repo.Listar(ctx)
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	for _, p := range catalogo {
		if p.Codigo == "TESTE-001" {
			t.Fatalf("produto desativado continua no catálogo: %+v", p)
		}
	}

	// ...mas continua encontrável, que é o que mantém imprimível uma nota
	// fiscal criada antes da desativação.
	desativado, err := repo.BuscarPorCodigo(ctx, "TESTE-001")
	if err != nil {
		t.Fatalf("produto desativado deveria continuar existindo: %v", err)
	}
	if desativado.Saldo != 20 {
		t.Fatalf("saldo do produto desativado mudou: %d", desativado.Saldo)
	}

	// Desativar duas vezes é erro: não há o que desativar.
	if err := repo.Desativar(ctx, "TESTE-001"); err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado na segunda desativação, obteve %v", err)
	}

	// Desativar inexistente
	if err := repo.Desativar(ctx, "NAO-EXISTE"); err != domain.ErrProdutoNaoEncontrado {
		t.Fatalf("esperava ErrProdutoNaoEncontrado, obteve %v", err)
	}

	// Cadastrar de novo o mesmo código reaproveita a linha desativada, em
	// vez de responder "código já cadastrado" para um produto invisível.
	recriado, err := repo.Criar(ctx, "TESTE-001", "Produto recriado", 7)
	if err != nil {
		t.Fatalf("recriar produto desativado: %v", err)
	}
	if recriado.Descricao != "Produto recriado" || recriado.Saldo != 7 {
		t.Fatalf("produto recriado com dados errados: %+v", recriado)
	}

	// E com o código já ativo, aí sim, duplicado.
	if _, err := repo.Criar(ctx, "TESTE-001", "Outro", 1); err != domain.ErrCodigoDuplicado {
		t.Fatalf("esperava ErrCodigoDuplicado, obteve %v", err)
	}
}

func TestProdutoRepository_SaldoNaoNegativo(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoProdutoRepository(pool)
	ctx := context.Background()

	if _, err := repo.Criar(ctx, "TESTE-002", "Produto de teste", 5); err != nil {
		t.Fatalf("Criar: %v", err)
	}

	// A constraint CHECK do banco deve impedir saldo negativo mesmo via
	// Atualizar (defesa em profundidade além da validação de serviço).
	_, err := repo.Atualizar(ctx, "TESTE-002", "Produto de teste", -1, nil)
	if err == nil {
		t.Fatalf("esperava erro ao tentar gravar saldo negativo")
	}
}

// TestProdutoRepository_AtualizarDetectaSaldoDesatualizado cobre o defeito
// em que um formulário aberto antes de uma impressão desfazia o débito ao
// salvar: a tela carregava saldo 10, a nota debitava 2, e o PUT gravava 10
// de volta — com a nota já Fechada.
func TestProdutoRepository_AtualizarDetectaSaldoDesatualizado(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoProdutoRepository(pool)
	ctx := context.Background()

	if _, err := repo.Criar(ctx, "CONF-001", "Parafuso", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	// O que a tela leu ao abrir o formulário.
	saldoNaTela := 10

	// Uma impressão debita 2 enquanto o formulário está aberto.
	if _, err := repo.Atualizar(ctx, "CONF-001", "Parafuso", 8, nil); err != nil {
		t.Fatalf("simular débito: %v", err)
	}

	// Salvar o formulário agora precisa ser recusado, não sobrescrever.
	_, err := repo.Atualizar(ctx, "CONF-001", "Parafuso editado", 10, &saldoNaTela)
	var desatualizado *domain.ErrSaldoDesatualizado
	if !errors.As(err, &desatualizado) {
		t.Fatalf("esperava ErrSaldoDesatualizado, obteve %v", err)
	}
	if desatualizado.Esperado != 10 || desatualizado.Atual != 8 {
		t.Fatalf("erro com valores errados: %+v", desatualizado)
	}

	produto, err := repo.BuscarPorCodigo(ctx, "CONF-001")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 8 {
		t.Fatalf("saldo foi sobrescrito: %d, esperado 8", produto.Saldo)
	}

	// Com o saldo corrente informado, a atualização passa.
	saldoAtual := 8
	atualizado, err := repo.Atualizar(ctx, "CONF-001", "Parafuso editado", 12, &saldoAtual)
	if err != nil {
		t.Fatalf("atualizar com saldo esperado correto: %v", err)
	}
	if atualizado.Saldo != 12 || atualizado.Descricao != "Parafuso editado" {
		t.Fatalf("produto atualizado inesperado: %+v", atualizado)
	}
}
