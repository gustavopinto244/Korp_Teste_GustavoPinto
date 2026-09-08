// Package apierror implementa o formato único de erro HTTP compartilhado
// pelos dois microsserviços do sistema (estoque e faturamento). O código é
// intencionalmente duplicado entre os serviços — eles não compartilham
// módulo Go.
package apierror

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

// Tipo de erro: "negocio" (usuário resolve) ou "sistema" (transitório).
const (
	TipoNegocio = "negocio"
	TipoSistema = "sistema"
)

// Erro é o corpo interno do envelope de erro.
type Erro struct {
	Codigo    string `json:"codigo"`
	Mensagem  string `json:"mensagem"`
	Tipo      string `json:"tipo"`
	Repetivel bool   `json:"repetivel"`
}

// Envelope é o formato de resposta JSON de erro devolvido por todos os
// endpoints do serviço.
type Envelope struct {
	Erro Erro `json:"erro"`
}

// Novo constrói um Envelope a partir dos campos do contrato único de erro.
func Novo(codigo, mensagem, tipo string, repetivel bool) Envelope {
	return Envelope{Erro: Erro{Codigo: codigo, Mensagem: mensagem, Tipo: tipo, Repetivel: repetivel}}
}

// Escrever serializa o Envelope como JSON no status HTTP informado.
func Escrever(w http.ResponseWriter, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		log.Printf("falha ao codificar envelope de erro: %v", err)
	}
}

// statusPara mapeia um erro de domínio conhecido para o par (status HTTP,
// Envelope) do contrato único de erro.
func statusPara(err error) (int, Envelope) {
	var erroEstoque *domain.ErroNegocioEstoque
	switch {
	case errors.As(err, &erroEstoque):
		status := http.StatusUnprocessableEntity
		switch erroEstoque.Codigo {
		case "PRODUTO_NAO_ENCONTRADO":
			status = http.StatusNotFound
		case "IDEMPOTENCY_KEY_CONFLITO":
			status = http.StatusConflict
		case "SALDO_INSUFICIENTE":
			status = http.StatusUnprocessableEntity
		default:
			if erroEstoque.Tipo == TipoSistema {
				status = http.StatusServiceUnavailable
			}
		}
		return status, Novo(erroEstoque.Codigo, erroEstoque.Mensagem, erroEstoque.Tipo, erroEstoque.Repetivel)

	case errors.Is(err, domain.ErrNotaNaoEncontrada):
		return http.StatusNotFound, Novo("NOTA_NAO_ENCONTRADA", "Nota fiscal não encontrada.", TipoNegocio, false)

	case errors.Is(err, domain.ErrNotaNaoAberta):
		return http.StatusConflict, Novo("NOTA_NAO_ABERTA", "Esta nota já foi fechada e não pode ser impressa novamente.", TipoNegocio, false)

	case errors.Is(err, domain.ErrEstoqueIndisponivel):
		return http.StatusServiceUnavailable, Novo("ESTOQUE_INDISPONIVEL", "Não foi possível confirmar a operação de estoque agora. Tente novamente em instantes.", TipoSistema, true)

	case errors.Is(err, domain.ErrProdutoInvalido):
		return http.StatusUnprocessableEntity, Novo("PRODUTO_INVALIDO", "Um ou mais produtos informados não existem no catálogo de estoque.", TipoNegocio, false)

	case errors.Is(err, domain.ErrIAIndisponivel):
		return http.StatusServiceUnavailable, Novo("IA_INDISPONIVEL", "Não foi possível interpretar o texto agora. Preencha os itens manualmente.", TipoSistema, true)

	default:
		// Erro inesperado: o texto e o `repetivel` são os mesmos usados pelo
		// serviço de estoque, para que o mesmo `codigo` chegue ao frontend
		// com exatamente o mesmo significado, venha de qual serviço vier.
		// `repetivel: true` porque um 500 costuma ser transitório
		// (indisponibilidade momentânea de banco, por exemplo) e repetir é
		// a orientação certa ao usuário.
		return http.StatusInternalServerError, Novo("ERRO_INTERNO", "Ocorreu um erro inesperado. Tente novamente em instantes.", TipoSistema, true)
	}
}

// EscreverErro traduz um erro de domínio (ou erro desconhecido) para o
// formato único de erro HTTP e escreve na resposta.
func EscreverErro(w http.ResponseWriter, err error) {
	status, env := statusPara(err)

	// O erro original nunca aparece no corpo da resposta (o contrato manda
	// mensagem pronta para o usuário, nunca stack trace), então sem este log
	// um 500 seria indebugável: o erro seria descartado silenciosamente.
	if status == http.StatusInternalServerError {
		log.Printf("erro interno: %v", err)
	}

	Escrever(w, status, env)
}

// EscreverValidacao escreve um erro de validação de entrada simples (422)
// com o código e mensagem informados.
func EscreverValidacao(w http.ResponseWriter, codigo, mensagem string) {
	Escrever(w, http.StatusUnprocessableEntity, Novo(codigo, mensagem, TipoNegocio, false))
}
