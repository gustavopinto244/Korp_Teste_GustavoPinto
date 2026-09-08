// Package apierror traduz erros de domínio para o formato único de erro
// HTTP compartilhado entre os microsserviços do sistema (ver plano técnico,
// seção 2.1).
package apierror

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
)

// ErroAPI é o conteúdo do campo "erro" no envelope de resposta.
type ErroAPI struct {
	Codigo    string `json:"codigo"`
	Mensagem  string `json:"mensagem"`
	Tipo      string `json:"tipo"`
	Repetivel bool   `json:"repetivel"`
}

// EnvelopeErro é o corpo JSON completo devolvido em respostas de erro.
type EnvelopeErro struct {
	Erro ErroAPI `json:"erro"`
}

const (
	TipoNegocio = "negocio"
	TipoSistema = "sistema"
)

// mapear converte um erro de domínio (ou um erro genérico/inesperado) no
// par (status HTTP, ErroAPI) correspondente ao contrato único de erro.
func mapear(err error) (int, ErroAPI) {
	var errSaldo *domain.ErrSaldoInsuficiente

	switch {
	case errors.Is(err, domain.ErrProdutoNaoEncontrado):
		return http.StatusNotFound, ErroAPI{
			Codigo:    "PRODUTO_NAO_ENCONTRADO",
			Mensagem:  "Produto não encontrado.",
			Tipo:      TipoNegocio,
			Repetivel: false,
		}

	case errors.Is(err, domain.ErrCodigoDuplicado):
		return http.StatusConflict, ErroAPI{
			Codigo:    "CODIGO_DUPLICADO",
			Mensagem:  "Já existe um produto cadastrado com este código.",
			Tipo:      TipoNegocio,
			Repetivel: false,
		}

	case errors.Is(err, domain.ErrValidacao):
		return http.StatusUnprocessableEntity, ErroAPI{
			Codigo:    "VALIDACAO",
			Mensagem:  err.Error(),
			Tipo:      TipoNegocio,
			Repetivel: false,
		}

	case errors.As(err, &errSaldo):
		return http.StatusUnprocessableEntity, ErroAPI{
			Codigo:    "SALDO_INSUFICIENTE",
			Mensagem:  errSaldo.Error(),
			Tipo:      TipoNegocio,
			Repetivel: false,
		}

	case errors.Is(err, domain.ErrChaveIdempotenciaConflitante):
		return http.StatusConflict, ErroAPI{
			Codigo:    "IDEMPOTENCY_KEY_CONFLITO",
			Mensagem:  "A chave de idempotência informada já foi usada com um conjunto de itens diferente.",
			Tipo:      TipoSistema,
			Repetivel: false,
		}

	case errors.Is(err, domain.ErrChaveIdempotenciaAusente):
		return http.StatusBadRequest, ErroAPI{
			Codigo:    "IDEMPOTENCY_KEY_AUSENTE",
			Mensagem:  "O header Idempotency-Key é obrigatório para esta operação.",
			Tipo:      TipoNegocio,
			Repetivel: false,
		}

	default:
		return http.StatusInternalServerError, ErroAPI{
			Codigo:    "ERRO_INTERNO",
			Mensagem:  "Ocorreu um erro inesperado. Tente novamente em instantes.",
			Tipo:      TipoSistema,
			Repetivel: true,
		}
	}
}

// EscreverErro mapeia err para o envelope único de erro e escreve a
// resposta HTTP correspondente (status + corpo JSON).
func EscreverErro(w http.ResponseWriter, err error) {
	status, erroAPI := mapear(err)

	if status == http.StatusInternalServerError {
		log.Printf("erro interno: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if encodeErr := json.NewEncoder(w).Encode(EnvelopeErro{Erro: erroAPI}); encodeErr != nil {
		log.Printf("falha ao codificar envelope de erro: %v", encodeErr)
	}
}

// StatusPara devolve apenas o status HTTP correspondente a err, sem
// escrever a resposta — útil quando o chamador precisa do status antes de
// montar o corpo (ex.: gravação na tabela de idempotência).
func StatusPara(err error) int {
	status, _ := mapear(err)
	return status
}

// EnvelopePara devolve o envelope de erro correspondente a err, sem
// escrever a resposta.
func EnvelopePara(err error) EnvelopeErro {
	_, erroAPI := mapear(err)
	return EnvelopeErro{Erro: erroAPI}
}
