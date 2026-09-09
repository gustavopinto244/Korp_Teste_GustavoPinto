package ia

// ATENÇÃO: este arquivo NÃO implementa inteligência artificial de verdade.
// É uma heurística determinística por regex/string matching (separação de
// cláusulas, extração de quantidade por dígito ou numeral por extenso, e
// comparação de substring contra as descrições do catálogo). Existe apenas
// para dar suporte ao endpoint POST /notas/interpretar nesta rodada, sem
// depender de uma chave de API externa. O nome do tipo é deliberadamente
// "mock" para não sugerir o contrário.

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// interpretadorMock implementa InterpretadorDeTexto com heurística
// determinística de string matching — não é um modelo de linguagem.
type interpretadorMock struct{}

// NovoInterpretadorMock constrói o interpretador heurístico determinístico.
func NovoInterpretadorMock() InterpretadorDeTexto {
	return &interpretadorMock{}
}

var reDigitos = regexp.MustCompile(`\d+`)

// numerosPorExtenso mapeia numerais por extenso simples (1 a 10) para seus
// valores. Cobre singular/masculino/feminino das formas mais comuns em
// português coloquial.
var numerosPorExtenso = map[string]int{
	"um": 1, "uma": 1,
	"dois": 2, "duas": 2,
	"tres": 3, "três": 3,
	"quatro": 4,
	"cinco":  5,
	"seis":   6,
	"sete":   7,
	"oito":   8,
	"nove":   9,
	"dez":    10,
}

// palavrasRuido são tokens removidos do restante da cláusula depois de
// extraída a quantidade, para sobrar só o que descreve o produto.
var palavrasRuido = map[string]bool{
	"unidades": true, "unidade": true, "de": true, "do": true, "da": true,
	"dos": true, "das": true, "o": true, "a": true, "os": true, "as": true,
}

func (i *interpretadorMock) Interpretar(_ context.Context, texto string, catalogo []CatalogoItem) (ResultadoInterpretacao, error) {
	resultado := ResultadoInterpretacao{
		ItensSugeridos:       []ItemSugerido{},
		ItensNaoReconhecidos: []ItemNaoReconhecido{},
	}

	for _, clausula := range dividirEmClausulas(texto) {
		clausulaOriginal := strings.TrimSpace(clausula)
		if clausulaOriginal == "" {
			continue
		}

		quantidade, restante, encontrouQuantidade := extrairQuantidade(clausulaOriginal)
		if !encontrouQuantidade {
			resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
				TextoOriginal: clausulaOriginal,
				Motivo:        "não foi possível identificar uma quantidade no texto",
			})
			continue
		}

		descricaoBusca := removerRuido(restante)
		item, encontrouProduto := casarComCatalogo(descricaoBusca, catalogo)
		if !encontrouProduto {
			resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
				TextoOriginal: clausulaOriginal,
				Motivo:        "produto não encontrado no catálogo",
			})
			continue
		}

		if item.ExcedeSaldo(quantidade) {
			// Mesmo teto aplicado à interpretação por modelo: o
			// comportamento da tela não pode depender de qual provider está
			// configurado.
			resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
				TextoOriginal: clausulaOriginal,
				Motivo: fmt.Sprintf(
					"quantidade solicitada (%d) acima do saldo disponível (%d)",
					quantidade, item.SaldoDisponivel,
				),
			})
			continue
		}

		resultado.ItensSugeridos = append(resultado.ItensSugeridos, ItemSugerido{
			ProdutoCodigo:    item.Codigo,
			ProdutoDescricao: item.Descricao,
			Quantidade:       quantidade,
			Confianca:        "alta",
		})
	}

	return resultado, nil
}

// dividirEmClausulas separa o texto livre em cláusulas independentes por
// " e " e vírgulas.
func dividirEmClausulas(texto string) []string {
	texto = strings.ReplaceAll(texto, ",", " e ")
	partes := strings.Split(texto, " e ")
	// também cobre "e" colado em quebras de linha ou início/fim de frase
	var resultado []string
	for _, p := range partes {
		resultado = append(resultado, strings.TrimSpace(p))
	}
	return resultado
}

// extrairQuantidade procura um número (dígito ou numeral por extenso) na
// cláusula e devolve a quantidade encontrada e o restante do texto sem o
// token de quantidade.
func extrairQuantidade(clausula string) (int, string, bool) {
	if loc := reDigitos.FindStringIndex(clausula); loc != nil {
		valor, err := strconv.Atoi(clausula[loc[0]:loc[1]])
		if err == nil {
			restante := clausula[:loc[0]] + " " + clausula[loc[1]:]
			return valor, restante, true
		}
	}

	palavras := strings.Fields(normalizar(clausula))
	for idx, p := range palavras {
		if valor, ok := numerosPorExtenso[p]; ok {
			// remove a palavra numeral do texto original (case-insensitive,
			// baseado na posição normalizada) reconstruindo sem ela.
			restante := strings.Join(append(append([]string{}, palavras[:idx]...), palavras[idx+1:]...), " ")
			return valor, restante, true
		}
	}

	return 0, "", false
}

// removerRuido tira artigos e expressões como "unidades de" do restante da
// cláusula, deixando só as palavras que descrevem o produto.
func removerRuido(restante string) string {
	palavras := strings.Fields(normalizar(restante))
	var filtradas []string
	for _, p := range palavras {
		if palavrasRuido[p] {
			continue
		}
		filtradas = append(filtradas, p)
	}
	return strings.Join(filtradas, " ")
}

// casarComCatalogo compara a descrição buscada (já normalizada, sem
// acentos, minúscula) contra a descrição de cada item do catálogo (também
// normalizada), em ambas as direções de strings.Contains. Nunca inventa um
// produto que não está no catálogo.
func casarComCatalogo(descricaoBusca string, catalogo []CatalogoItem) (CatalogoItem, bool) {
	descricaoBusca = strings.TrimSpace(descricaoBusca)
	if descricaoBusca == "" {
		return CatalogoItem{}, false
	}

	for _, item := range catalogo {
		descricaoCatalogo := normalizar(item.Descricao)
		if strings.Contains(descricaoCatalogo, descricaoBusca) || strings.Contains(descricaoBusca, descricaoCatalogo) {
			return item, true
		}

		// casamento por palavras: todas as palavras da busca aparecem na
		// descrição do catálogo (cobre "parafusos sextavados" vs "parafuso
		// sextavado m6").
		if todasPalavrasContidas(descricaoBusca, descricaoCatalogo) {
			return item, true
		}
	}
	return CatalogoItem{}, false
}

func todasPalavrasContidas(busca, catalogo string) bool {
	palavrasBusca := strings.Fields(busca)
	if len(palavrasBusca) == 0 {
		return false
	}
	for _, p := range palavrasBusca {
		p = strings.TrimSuffix(p, "s") // singular/plural simples
		if !strings.Contains(catalogo, p) {
			return false
		}
	}
	return true
}

var substituicoesAcento = strings.NewReplacer(
	"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c",
	"Á", "a", "À", "a", "Ã", "a", "Â", "a",
	"É", "e", "È", "e", "Ê", "e",
	"Í", "i", "Ó", "o", "Õ", "o", "Ô", "o",
	"Ú", "u", "Ç", "c",
)

// normalizar coloca em minúsculas e remove acentos comuns do português,
// para comparação robusta independente de acentuação.
func normalizar(s string) string {
	return substituicoesAcento.Replace(strings.ToLower(s))
}
