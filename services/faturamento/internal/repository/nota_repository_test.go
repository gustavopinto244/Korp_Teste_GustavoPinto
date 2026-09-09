package repository_test

import (
	"context"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/testdb"
)

func TestCriar_NumeracaoSequencialSemDuplicar(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	nota1, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso", Quantidade: 2}})
	if err != nil {
		t.Fatalf("criar nota 1: %v", err)
	}
	nota2, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "MART-002", ProdutoDescricao: "Martelo", Quantidade: 1}})
	if err != nil {
		t.Fatalf("criar nota 2: %v", err)
	}

	if nota1.Numero == nota2.Numero {
		t.Fatalf("números não deveriam se repetir: %d == %d", nota1.Numero, nota2.Numero)
	}
	if nota2.Numero != nota1.Numero+1 {
		t.Fatalf("esperava numeração sequencial estrita, obteve %d depois de %d", nota2.Numero, nota1.Numero)
	}
	if nota1.Status != domain.StatusAberta {
		t.Fatalf("nota deveria nascer Aberta, veio %s", nota1.Status)
	}
	if len(nota1.Itens) != 1 || nota1.Itens[0].ProdutoCodigo != "PARAF-001" {
		t.Fatalf("itens não persistidos corretamente: %+v", nota1.Itens)
	}
}

func TestBuscarPorID_NaoEncontrada(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	_, err := repo.BuscarPorID(ctx, 99999)
	if err != domain.ErrNotaNaoEncontrada {
		t.Fatalf("esperava ErrNotaNaoEncontrada, obteve %v", err)
	}
}

func TestMarcarFechada_RespeitaWhereStatusAberta(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	nota, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso", Quantidade: 1}})
	if err != nil {
		t.Fatalf("criar nota: %v", err)
	}

	fechada, err := repo.MarcarFechada(ctx, nota.ID)
	if err != nil {
		t.Fatalf("marcar fechada: %v", err)
	}
	if fechada.Status != domain.StatusFechada {
		t.Fatalf("esperava status Fechada, obteve %s", fechada.Status)
	}
	if fechada.FechadoEm == nil {
		t.Fatal("esperava fechado_em preenchido")
	}

	// segunda tentativa: nota já não está Aberta, deve falhar
	_, err = repo.MarcarFechada(ctx, nota.ID)
	if err != domain.ErrNotaNaoAberta {
		t.Fatalf("esperava ErrNotaNaoAberta na segunda tentativa, obteve %v", err)
	}
}

func TestListar_DevolveTodasAsNotas(t *testing.T) {
	pool := testdb.AbrirPool(t)
	repo := repository.NovoNotaRepository(pool)
	ctx := context.Background()

	_, err := repo.Criar(ctx, []domain.ItemNota{{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso", Quantidade: 1}})
	if err != nil {
		t.Fatalf("criar nota: %v", err)
	}

	notas, err := repo.Listar(ctx)
	if err != nil {
		t.Fatalf("listar notas: %v", err)
	}
	if len(notas) == 0 {
		t.Fatal("esperava ao menos uma nota listada")
	}
}
