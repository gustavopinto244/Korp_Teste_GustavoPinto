package ia_test

import (
	"context"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/ia"
)

func catalogoDeTeste() []ia.CatalogoItem {
	return []ia.CatalogoItem{
		{Codigo: "PARAF-001", Descricao: "Parafuso sextavado M6"},
		{Codigo: "MART-002", Descricao: "Martelo de borracha"},
		{Codigo: "DISC-003", Descricao: "Disco de corte"},
	}
}

func TestInterpretadorMock_ReconheceQuantidadesNumericasEPorExtenso(t *testing.T) {
	casos := []struct {
		nome              string
		texto             string
		esperaCodigos     []string
		esperaQuantidades []int
	}{
		{
			nome:              "quantidades numéricas",
			texto:             "3 parafusos sextavados e 2 martelos",
			esperaCodigos:     []string{"PARAF-001", "MART-002"},
			esperaQuantidades: []int{3, 2},
		},
		{
			nome:              "quantidade por extenso",
			texto:             "dois martelos de borracha",
			esperaCodigos:     []string{"MART-002"},
			esperaQuantidades: []int{2},
		},
		{
			nome:              "um por extenso",
			texto:             "um parafuso sextavado",
			esperaCodigos:     []string{"PARAF-001"},
			esperaQuantidades: []int{1},
		},
		{
			nome:              "com vírgula",
			texto:             "3 parafusos sextavados, 1 disco de corte",
			esperaCodigos:     []string{"PARAF-001", "DISC-003"},
			esperaQuantidades: []int{3, 1},
		},
	}

	interpretador := ia.NovoInterpretadorMock()

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			resultado, err := interpretador.Interpretar(context.Background(), c.texto, catalogoDeTeste())
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(resultado.ItensSugeridos) != len(c.esperaCodigos) {
				t.Fatalf("esperava %d itens sugeridos, obteve %d: %+v", len(c.esperaCodigos), len(resultado.ItensSugeridos), resultado.ItensSugeridos)
			}
			for idx, item := range resultado.ItensSugeridos {
				if item.ProdutoCodigo != c.esperaCodigos[idx] {
					t.Errorf("item %d: esperava código %s, obteve %s", idx, c.esperaCodigos[idx], item.ProdutoCodigo)
				}
				if item.Quantidade != c.esperaQuantidades[idx] {
					t.Errorf("item %d: esperava quantidade %d, obteve %d", idx, c.esperaQuantidades[idx], item.Quantidade)
				}
			}
		})
	}
}

func TestInterpretadorMock_IgnoraProdutoNaoReconhecido(t *testing.T) {
	interpretador := ia.NovoInterpretadorMock()
	resultado, err := interpretador.Interpretar(context.Background(), "5 unidades de parafuso alien inexistente", catalogoDeTeste())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(resultado.ItensSugeridos) != 0 {
		t.Fatalf("não esperava itens sugeridos, obteve %+v", resultado.ItensSugeridos)
	}
	if len(resultado.ItensNaoReconhecidos) != 1 {
		t.Fatalf("esperava 1 item não reconhecido, obteve %+v", resultado.ItensNaoReconhecidos)
	}
}

func TestInterpretadorMock_NuncaInventaProdutoForaDoCatalogo(t *testing.T) {
	interpretador := ia.NovoInterpretadorMock()
	resultado, err := interpretador.Interpretar(context.Background(), "10 parafusos e 3 martelos e 2 discos de corte e 1 chave de fenda", catalogoDeTeste())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	for _, item := range resultado.ItensSugeridos {
		encontrado := false
		for _, c := range catalogoDeTeste() {
			if c.Codigo == item.ProdutoCodigo {
				encontrado = true
			}
		}
		if !encontrado {
			t.Fatalf("item sugerido com código fora do catálogo: %+v", item)
		}
	}
	// "chave de fenda" não está no catálogo de teste, deve cair em não reconhecidos
	achouChaveDeFenda := false
	for _, nr := range resultado.ItensNaoReconhecidos {
		if nr.TextoOriginal == "1 chave de fenda" {
			achouChaveDeFenda = true
		}
	}
	if !achouChaveDeFenda {
		t.Fatalf("esperava 'chave de fenda' em não reconhecidos, obteve %+v", resultado.ItensNaoReconhecidos)
	}
}

func TestInterpretadorMock_SemQuantidadeVaiParaNaoReconhecidos(t *testing.T) {
	interpretador := ia.NovoInterpretadorMock()
	resultado, err := interpretador.Interpretar(context.Background(), "parafuso sextavado", catalogoDeTeste())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(resultado.ItensSugeridos) != 0 {
		t.Fatalf("não esperava sugestão sem quantidade, obteve %+v", resultado.ItensSugeridos)
	}
	if len(resultado.ItensNaoReconhecidos) != 1 {
		t.Fatalf("esperava 1 item não reconhecido, obteve %+v", resultado.ItensNaoReconhecidos)
	}
}
