package domain

import "errors"

// Erros sentinela de domínio, traduzidos para o formato único de erro HTTP
// pela camada internal/apierror.
var (
	// ErrNotaNaoEncontrada indica que o ID de nota informado não existe.
	ErrNotaNaoEncontrada = errors.New("nota fiscal não encontrada")

	// ErrNotaNaoAberta indica que a operação de impressão foi tentada numa
	// nota que já não está no status Aberta.
	ErrNotaNaoAberta = errors.New("nota fiscal não está aberta")

	// ErrEstoqueIndisponivel indica que o serviço de estoque não respondeu
	// dentro da política de resiliência (timeout, conexão recusada, 5xx
	// repetidos, ou circuit breaker aberto).
	ErrEstoqueIndisponivel = errors.New("serviço de estoque indisponível")

	// ErrProdutoInvalido indica que um item referencia um código de produto
	// que o estoque não reconhece.
	ErrProdutoInvalido = errors.New("produto inválido ou inexistente no catálogo de estoque")

	// ErrIAIndisponivel indica que o interpretador de texto livre falhou ou
	// não está configurado corretamente.
	ErrIAIndisponivel = errors.New("serviço de interpretação de texto indisponível")
)

// ErroNegocioEstoque representa um erro de negócio devolvido pelo serviço de
// estoque (ex.: saldo insuficiente, produto não encontrado, conflito de
// idempotência) que deve ser propagado ao chamador do faturamento com o
// mesmo código/mensagem/tipo/repetível recebidos, sem tradução.
type ErroNegocioEstoque struct {
	Codigo    string
	Mensagem  string
	Tipo      string
	Repetivel bool
}

func (e *ErroNegocioEstoque) Error() string {
	return e.Mensagem
}
