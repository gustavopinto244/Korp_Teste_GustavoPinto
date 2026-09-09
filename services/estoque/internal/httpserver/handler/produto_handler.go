// Package handler contém os handlers HTTP do serviço de estoque. Traduzem
// requisições/respostas JSON para chamadas ao service, e erros de domínio
// para o formato único de erro via apierror.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
)

// ProdutoHandler expõe o CRUD de produtos via HTTP.
type ProdutoHandler struct {
	svc *service.ProdutoService
}

// NovoProdutoHandler cria um ProdutoHandler sobre o service informado.
func NovoProdutoHandler(svc *service.ProdutoService) *ProdutoHandler {
	return &ProdutoHandler{svc: svc}
}

type produtoRequest struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	Saldo     int    `json:"saldo"`
	// SaldoEsperado é opcional e só vale no PUT: o saldo que o cliente viu
	// quando abriu o formulário. Informado, faz a atualização ser recusada
	// com 409 se o saldo já tiver mudado — tipicamente porque uma nota foi
	// impressa nesse meio-tempo. Ausente, o valor enviado sobrescreve.
	SaldoEsperado *int `json:"saldoEsperado,omitempty"`
}

type produtoResponse struct {
	ID        int64  `json:"id"`
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	Saldo     int    `json:"saldo"`
}

func paraResposta(p *domain.Produto) produtoResponse {
	return produtoResponse{ID: p.ID, Codigo: p.Codigo, Descricao: p.Descricao, Saldo: p.Saldo}
}

func escreverJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

// Criar trata POST /produtos.
func (h *ProdutoHandler) Criar(w http.ResponseWriter, r *http.Request) {
	var req produtoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.EscreverErro(w, domain.ErrValidacao)
		return
	}

	produto, err := h.svc.Criar(r.Context(), req.Codigo, req.Descricao, req.Saldo)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	escreverJSON(w, http.StatusCreated, paraResposta(produto))
}

// Listar trata GET /produtos.
func (h *ProdutoHandler) Listar(w http.ResponseWriter, r *http.Request) {
	produtos, err := h.svc.Listar(r.Context())
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	resposta := make([]produtoResponse, 0, len(produtos))
	for i := range produtos {
		resposta = append(resposta, paraResposta(&produtos[i]))
	}

	escreverJSON(w, http.StatusOK, resposta)
}

// Detalhe trata GET /produtos/{codigo}.
func (h *ProdutoHandler) Detalhe(w http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")

	produto, err := h.svc.BuscarPorCodigo(r.Context(), codigo)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	escreverJSON(w, http.StatusOK, paraResposta(produto))
}

// Atualizar trata PUT /produtos/{codigo}.
func (h *ProdutoHandler) Atualizar(w http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")

	var req produtoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.EscreverErro(w, domain.ErrValidacao)
		return
	}

	produto, err := h.svc.Atualizar(r.Context(), codigo, req.Descricao, req.Saldo, req.SaldoEsperado)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	escreverJSON(w, http.StatusOK, paraResposta(produto))
}

// Remover trata DELETE /produtos/{codigo}.
func (h *ProdutoHandler) Remover(w http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")

	if err := h.svc.Remover(r.Context(), codigo); err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
