// Package ia contém o ponto de extensão de interpretação de texto livre
// usado pelo endpoint POST /notas/interpretar. A implementação ativa nesta
// rodada é um mock determinístico (ver mock_interpretador.go) — não é um
// modelo de linguagem real.
package ia

import "context"

// CatalogoItem é a projeção mínima do catálogo de produtos do estoque
// necessária para a interpretação: código e descrição.
type CatalogoItem struct {
	Codigo    string
	Descricao string
}

// ItemSugerido é um item que o interpretador reconheceu no texto livre e
// conseguiu casar contra o catálogo.
type ItemSugerido struct {
	ProdutoCodigo    string `json:"produtoCodigo"`
	ProdutoDescricao string `json:"produtoDescricao"`
	Quantidade       int    `json:"quantidade"`
	Confianca        string `json:"confianca"`
}

// ItemNaoReconhecido é um trecho do texto livre que não pôde ser casado
// contra nenhum item do catálogo.
type ItemNaoReconhecido struct {
	TextoOriginal string `json:"textoOriginal"`
	Motivo        string `json:"motivo"`
}

// ResultadoInterpretacao é a saída estruturada devolvida por
// POST /notas/interpretar.
type ResultadoInterpretacao struct {
	ItensSugeridos       []ItemSugerido       `json:"itensSugeridos"`
	ItensNaoReconhecidos []ItemNaoReconhecido `json:"itensNaoReconhecidos"`
}

// InterpretadorDeTexto é o ponto de extensão para interpretação de texto
// livre em itens de nota fiscal estruturados. Implementações nunca inventam
// produtos que não estão no catálogo informado.
type InterpretadorDeTexto interface {
	Interpretar(ctx context.Context, texto string, catalogo []CatalogoItem) (ResultadoInterpretacao, error)
}

// NovoInterpretador escolhe a implementação de InterpretadorDeTexto pela
// variável de ambiente IA_PROVIDER ("mock" ou "claude"). Qualquer valor
// desconhecido ou vazio cai no mock determinístico, para nunca deixar o
// endpoint sem implementação.
func NovoInterpretador(provider, apiKey string) InterpretadorDeTexto {
	switch provider {
	case "claude":
		return NovoInterpretadorClaude(apiKey)
	default:
		return NovoInterpretadorMock()
	}
}
