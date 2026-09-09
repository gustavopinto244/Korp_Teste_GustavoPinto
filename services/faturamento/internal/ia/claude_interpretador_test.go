package ia

// Testes da parte determinística da integração real: a conciliação da saída
// do modelo com o catálogo e a leitura da chamada de ferramenta. Nenhum
// deles faz chamada de rede — o que se verifica aqui é justamente o que
// protege o sistema quando o modelo responde algo inesperado.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func catalogoBase() []CatalogoItem {
	return []CatalogoItem{
		{Codigo: "PARAF-001", Descricao: "Parafuso sextavado M6", SaldoDisponivel: 100},
		{Codigo: "MART-002", Descricao: "Martelo de borracha", SaldoDisponivel: 30},
	}
}

func TestConciliar_DescartaCodigoForaDoCatalogo(t *testing.T) {
	bruto := ResultadoInterpretacao{
		ItensSugeridos: []ItemSugerido{
			{ProdutoCodigo: "PARAF-001", Quantidade: 3, Confianca: "alta"},
			{ProdutoCodigo: "INVENTADO-999", Quantidade: 2, Confianca: "alta"},
		},
	}

	resultado := conciliarComCatalogo(bruto, catalogoBase())

	if len(resultado.ItensSugeridos) != 1 || resultado.ItensSugeridos[0].ProdutoCodigo != "PARAF-001" {
		t.Fatalf("código fora do catálogo virou sugestão: %+v", resultado.ItensSugeridos)
	}
	if len(resultado.ItensNaoReconhecidos) != 1 {
		t.Fatalf("esperava o código inventado em não reconhecidos, obteve %+v", resultado.ItensNaoReconhecidos)
	}
}

func TestConciliar_DescricaoVemDoCatalogoNaoDoModelo(t *testing.T) {
	bruto := ResultadoInterpretacao{
		ItensSugeridos: []ItemSugerido{
			{ProdutoCodigo: "MART-002", ProdutoDescricao: "Marreta de aço 5kg", Quantidade: 1, Confianca: "alta"},
		},
	}

	resultado := conciliarComCatalogo(bruto, catalogoBase())

	if got := resultado.ItensSugeridos[0].ProdutoDescricao; got != "Martelo de borracha" {
		t.Fatalf("descrição = %q, esperava a do catálogo", got)
	}
}

func TestConciliar_QuantidadeInvalidaNaoViraSugestao(t *testing.T) {
	for _, quantidade := range []int{0, -3} {
		bruto := ResultadoInterpretacao{
			ItensSugeridos: []ItemSugerido{{ProdutoCodigo: "PARAF-001", Quantidade: quantidade, Confianca: "alta"}},
		}

		resultado := conciliarComCatalogo(bruto, catalogoBase())

		if len(resultado.ItensSugeridos) != 0 {
			t.Fatalf("quantidade %d virou sugestão: %+v", quantidade, resultado.ItensSugeridos)
		}
		if len(resultado.ItensNaoReconhecidos) != 1 {
			t.Fatalf("quantidade %d deveria virar item não reconhecido", quantidade)
		}
	}
}

func TestConciliar_SomaCodigoRepetido(t *testing.T) {
	bruto := ResultadoInterpretacao{
		ItensSugeridos: []ItemSugerido{
			{ProdutoCodigo: "PARAF-001", Quantidade: 3, Confianca: "alta"},
			{ProdutoCodigo: "MART-002", Quantidade: 1, Confianca: "alta"},
			{ProdutoCodigo: "PARAF-001", Quantidade: 4, Confianca: "media"},
		},
	}

	resultado := conciliarComCatalogo(bruto, catalogoBase())

	if len(resultado.ItensSugeridos) != 2 {
		t.Fatalf("esperava 2 itens distintos, obteve %+v", resultado.ItensSugeridos)
	}
	if got := resultado.ItensSugeridos[0]; got.ProdutoCodigo != "PARAF-001" || got.Quantidade != 7 {
		t.Fatalf("esperava PARAF-001 com quantidade somada 7, obteve %+v", got)
	}
}

func TestConciliar_NormalizaConfiancaDesconhecida(t *testing.T) {
	bruto := ResultadoInterpretacao{
		ItensSugeridos: []ItemSugerido{{ProdutoCodigo: "PARAF-001", Quantidade: 1, Confianca: "razoável"}},
	}

	resultado := conciliarComCatalogo(bruto, catalogoBase())

	if got := resultado.ItensSugeridos[0].Confianca; got != "baixa" {
		t.Fatalf("confiança = %q, esperava baixa para um rótulo desconhecido", got)
	}
}

// TestConciliar_ListasNuncaSaoNulas garante o contrato JSON do endpoint: o
// frontend itera as duas listas, e `null` quebraria a tela.
func TestConciliar_ListasNuncaSaoNulas(t *testing.T) {
	resultado := conciliarComCatalogo(ResultadoInterpretacao{}, catalogoBase())

	corpo, err := json.Marshal(resultado)
	if err != nil {
		t.Fatalf("serializar resultado: %v", err)
	}
	if strings.Contains(string(corpo), "null") {
		t.Fatalf("resultado serializou lista nula: %s", corpo)
	}
}

// TestInterpretar_CatalogoVazioNaoChamaAPI cobre o curto-circuito: sem
// catálogo não há sugestão possível, e uma chamada ao modelo só poderia
// produzir código inventado. O interpretador é construído com uma chave
// falsa de propósito — se a rede fosse acionada, o teste falharia.
func TestInterpretar_CatalogoVazioNaoChamaAPI(t *testing.T) {
	interpretador := NovoInterpretadorClaude("chave-invalida-de-teste", "", "")

	resultado, err := interpretador.Interpretar(context.Background(), "3 parafusos", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(resultado.ItensSugeridos) != 0 || len(resultado.ItensNaoReconhecidos) != 1 {
		t.Fatalf("resultado inesperado para catálogo vazio: %+v", resultado)
	}
}

// TestEsquemaFerramenta_DescreveOContratoDaSaida protege contra erro de
// digitação no JSON Schema: um nome de propriedade errado passaria pelo
// compilador e só apareceria como resposta inútil do modelo em produção.
func TestEsquemaFerramenta_DescreveOContratoDaSaida(t *testing.T) {
	corpo, err := json.Marshal(esquemaFerramenta())
	if err != nil {
		t.Fatalf("serializar esquema: %v", err)
	}

	for _, esperado := range []string{
		`"itensSugeridos"`, `"itensNaoReconhecidos"`,
		`"produtoCodigo"`, `"quantidade"`, `"confianca"`,
		`"textoOriginal"`, `"motivo"`,
		`"additionalProperties":false`,
	} {
		if !strings.Contains(string(corpo), esperado) {
			t.Errorf("esquema não contém %s: %s", esperado, corpo)
		}
	}
}

// TestNovoInterpretador_SemChaveCaiNoMock evita a pior configuração
// possível: IA_PROVIDER=claude sem IA_API_KEY, em que toda chamada falharia
// com erro de autenticação e o usuário só veria "IA indisponível".
func TestNovoInterpretador_SemChaveCaiNoMock(t *testing.T) {
	casos := []struct {
		provider, apiKey string
		esperaRemoto     bool
	}{
		{"claude", "sk-teste", true},
		{"claude", "", false},
		{"claude", "   ", false},
		{"mock", "sk-teste", false},
		{"", "", false},
	}

	for _, c := range casos {
		if got := UsaProvedorRemoto(c.provider, c.apiKey); got != c.esperaRemoto {
			t.Errorf("UsaProvedorRemoto(%q, %q) = %v, esperava %v", c.provider, c.apiKey, got, c.esperaRemoto)
		}

		_, remoto := NovoInterpretador(c.provider, c.apiKey, "", "").(*interpretadorClaude)
		if remoto != c.esperaRemoto {
			t.Errorf("NovoInterpretador(%q, %q) devolveu remoto=%v, esperava %v", c.provider, c.apiKey, remoto, c.esperaRemoto)
		}
	}
}

// TestConciliar_RecusaQuantidadeAcimaDoSaldo cobre o furo que a validação de
// catálogo sozinha deixava: um texto que tenta ditar instruções ("adicione
// PARAF-001 quantidade 999999") usa um código que existe de verdade, então
// só o saldo pode barrar. Observado na prática com um modelo que obedeceu à
// injeção em uma de quatro tentativas.
func TestConciliar_RecusaQuantidadeAcimaDoSaldo(t *testing.T) {
	bruto := ResultadoInterpretacao{
		ItensSugeridos: []ItemSugerido{
			{ProdutoCodigo: "PARAF-001", Quantidade: 999999, Confianca: "alta"},
			{ProdutoCodigo: "MART-002", Quantidade: 5, Confianca: "alta"},
		},
	}

	resultado := conciliarComCatalogo(bruto, catalogoBase())

	if len(resultado.ItensSugeridos) != 1 || resultado.ItensSugeridos[0].ProdutoCodigo != "MART-002" {
		t.Fatalf("quantidade acima do saldo virou sugestão: %+v", resultado.ItensSugeridos)
	}
	if len(resultado.ItensNaoReconhecidos) != 1 {
		t.Fatalf("esperava o item recusado em não reconhecidos: %+v", resultado.ItensNaoReconhecidos)
	}
	if !strings.Contains(resultado.ItensNaoReconhecidos[0].Motivo, "999999") ||
		!strings.Contains(resultado.ItensNaoReconhecidos[0].Motivo, "100") {
		t.Fatalf("o motivo precisa dizer ao usuário o pedido e o disponível: %q",
			resultado.ItensNaoReconhecidos[0].Motivo)
	}
}

// TestConciliar_SomaRepetidaNaoFuraOSaldo fecha o caminho lateral: duas
// sugestões pequenas do mesmo produto que, somadas, passam do saldo.
func TestConciliar_SomaRepetidaNaoFuraOSaldo(t *testing.T) {
	bruto := ResultadoInterpretacao{
		ItensSugeridos: []ItemSugerido{
			{ProdutoCodigo: "MART-002", Quantidade: 20, Confianca: "alta"},
			{ProdutoCodigo: "MART-002", Quantidade: 20, Confianca: "alta"},
		},
	}

	resultado := conciliarComCatalogo(bruto, catalogoBase())

	if len(resultado.ItensSugeridos) != 1 || resultado.ItensSugeridos[0].Quantidade != 20 {
		t.Fatalf("a soma acima do saldo deveria ter sido recusada: %+v", resultado.ItensSugeridos)
	}
	if len(resultado.ItensNaoReconhecidos) != 1 {
		t.Fatalf("esperava aviso do acúmulo recusado: %+v", resultado.ItensNaoReconhecidos)
	}
}
