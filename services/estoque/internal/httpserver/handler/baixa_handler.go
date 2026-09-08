package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
)

// BaixaHandler expõe o endpoint mais importante do serviço: a baixa
// atômica e idempotente de saldo de múltiplos produtos.
type BaixaHandler struct {
	svc *service.BaixaService
}

// NovoBaixaHandler cria um BaixaHandler sobre o service informado.
func NovoBaixaHandler(svc *service.BaixaService) *BaixaHandler {
	return &BaixaHandler{svc: svc}
}

// Processar trata POST /produtos/baixa.
func (h *BaixaHandler) Processar(w http.ResponseWriter, r *http.Request) {
	chave := r.Header.Get("Idempotency-Key")
	if chave == "" {
		apierror.EscreverErro(w, domain.ErrChaveIdempotenciaAusente)
		return
	}

	var req service.RequisicaoBaixa
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.EscreverErro(w, domain.ErrValidacao)
		return
	}

	resposta, err := h.svc.Processar(r.Context(), chave, req)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	escreverJSON(w, http.StatusOK, resposta)
}
