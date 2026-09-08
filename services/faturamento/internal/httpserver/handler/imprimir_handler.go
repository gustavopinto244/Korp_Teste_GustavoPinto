package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

// ImprimirServico é o subconjunto de internal/service.ImprimirService usado
// pelo handler de impressão.
type ImprimirServico interface {
	Imprimir(ctx context.Context, notaID int64) (domain.NotaFiscal, error)
}

// ImprimirHandler trata POST /notas/{id}/imprimir — o endpoint central do
// sistema, ver internal/service/imprimir_service.go para o fluxo completo.
type ImprimirHandler struct {
	servico ImprimirServico
}

// NovoImprimirHandler constrói um ImprimirHandler.
func NovoImprimirHandler(servico ImprimirServico) *ImprimirHandler {
	return &ImprimirHandler{servico: servico}
}

// Imprimir trata POST /notas/{id}/imprimir.
func (h *ImprimirHandler) Imprimir(w http.ResponseWriter, r *http.Request) {
	id, err := idDaRota(r)
	if err != nil {
		apierror.EscreverValidacao(w, "ID_INVALIDO", "O ID da nota deve ser um número inteiro.")
		return
	}

	nota, err := h.servico.Imprimir(r.Context(), id)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(notaParaJSON(nota))
}
