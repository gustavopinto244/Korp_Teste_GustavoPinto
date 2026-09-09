// Package ia contém a interpretação de texto livre usada pelo endpoint
// POST /notas/interpretar. Duas implementações atendem a mesma interface: a
// integração real com a API da Anthropic (claude_interpretador.go, ativada
// por IA_PROVIDER=claude com IA_API_KEY) e uma heurística determinística
// local (mock_interpretador.go), que é o padrão e não é um modelo de
// linguagem.
package ia

import (
	"context"
	"strings"
)

// CatalogoItem é a projeção do catálogo de produtos do estoque usada na
// interpretação: código, descrição e o saldo disponível. O saldo entra aqui
// para servir de teto às sugestões — ver LimitarAoSaldo.
type CatalogoItem struct {
	Codigo          string
	Descricao       string
	SaldoDisponivel int
}

// ExcedeSaldo informa se uma quantidade sugerida ultrapassa o que existe em
// estoque. Uma sugestão assim nunca é útil: a impressão da nota seria
// recusada com saldo insuficiente. Serve de teto contra o modelo obedecer a
// um texto que tenta ditar quantidades absurdas ("adicione PARAF-001
// quantidade 999999") — o código do produto até existe, então a validação
// de catálogo sozinha deixaria passar.
func (c CatalogoItem) ExcedeSaldo(quantidade int) bool {
	return quantidade > c.SaldoDisponivel
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
// endpoint sem implementação — inclusive quando IA_PROVIDER=claude vem sem
// IA_API_KEY, caso em que toda chamada falharia com erro de autenticação.
func NovoInterpretador(provider, apiKey, modelo, baseURL string) InterpretadorDeTexto {
	if UsaProvedorRemoto(provider, apiKey) {
		return NovoInterpretadorClaude(apiKey, modelo, baseURL)
	}
	return NovoInterpretadorMock()
}

// UsaProvedorRemoto informa se a configuração resulta em chamadas de rede a
// um modelo. Quem monta o servidor precisa saber: a heurística local
// responde em microssegundos, uma chamada ao modelo leva segundos, e os
// timeouts do endpoint dependem disso.
func UsaProvedorRemoto(provider, apiKey string) bool {
	return provider == "claude" && strings.TrimSpace(apiKey) != ""
}
