package domain

import (
	"errors"
	"fmt"
)

// ErrProdutoNaoEncontrado é retornado quando um código de produto informado
// não existe no catálogo.
var ErrProdutoNaoEncontrado = errors.New("produto não encontrado")

// ErrCodigoDuplicado é retornado ao tentar criar um produto com um código
// que já existe (violação da constraint uq_produto_codigo).
var ErrCodigoDuplicado = errors.New("código de produto já cadastrado")

// ErrValidacao é retornado quando os dados de entrada de um produto são
// inválidos (campos obrigatórios vazios, saldo negativo etc).
var ErrValidacao = errors.New("dados de produto inválidos")

// ErrChaveIdempotenciaConflitante é retornado quando a mesma chave de
// idempotência chega com um payload de itens diferente do que gerou o
// resultado originalmente salvo. Situação anômala, não esperada no fluxo
// normal (ver plano técnico, seção 2.4).
var ErrChaveIdempotenciaConflitante = errors.New("chave de idempotência já usada com payload diferente")

// ErrChaveIdempotenciaAusente é retornado quando o header Idempotency-Key
// obrigatório não foi enviado na requisição de baixa.
var ErrChaveIdempotenciaAusente = errors.New("header Idempotency-Key é obrigatório")

// ErrSaldoInsuficiente é retornado quando uma baixa solicita mais unidades
// de um produto do que o saldo disponível. Carrega os dados necessários
// para montar a mensagem de erro exigida pelo contrato (código do produto,
// saldo disponível e quantidade solicitada).
type ErrSaldoInsuficiente struct {
	Codigo     string
	Disponivel int
	Solicitado int
}

func (e *ErrSaldoInsuficiente) Error() string {
	return fmt.Sprintf(
		"Saldo insuficiente para o produto %s. Disponível: %d, solicitado: %d.",
		e.Codigo, e.Disponivel, e.Solicitado,
	)
}

// Is permite usar errors.Is(err, ErrSaldoInsuficienteSentinela) de forma
// idiomática mesmo com o valor carregando dados variáveis, comparando
// apenas o tipo do erro.
func (e *ErrSaldoInsuficiente) Is(target error) bool {
	_, ok := target.(*ErrSaldoInsuficiente)
	return ok
}
