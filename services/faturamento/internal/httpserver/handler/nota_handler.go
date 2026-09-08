// Package handler implementa os handlers HTTP do serviço de faturamento,
// traduzindo requisições/respostas JSON para chamadas ao internal/service e
// erros de domínio para o formato único de erro via internal/apierror.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

// NotaServico é o subconjunto de internal/service.NotaService usado pelos
// handlers de nota.
type NotaServico interface {
	Criar(ctx context.Context, entradas []domain.ItemEntrada) (domain.NotaFiscal, error)
	Listar(ctx context.Context) ([]domain.NotaFiscal, error)
	BuscarPorID(ctx context.Context, id int64) (domain.NotaFiscal, error)
}

// NotaHandler agrupa os handlers HTTP de CRUD de notas fiscais.
type NotaHandler struct {
	servico NotaServico
}

// NovoNotaHandler constrói um NotaHandler.
func NovoNotaHandler(servico NotaServico) *NotaHandler {
	return &NotaHandler{servico: servico}
}

type itemEntradaJSON struct {
	ProdutoCodigo string `json:"produtoCodigo"`
	Quantidade    int    `json:"quantidade"`
}

type criarNotaRequest struct {
	Itens []itemEntradaJSON `json:"itens"`
}

type itemRespostaJSON struct {
	ProdutoCodigo    string `json:"produtoCodigo"`
	ProdutoDescricao string `json:"produtoDescricao"`
	Quantidade       int    `json:"quantidade"`
}

type notaRespostaJSON struct {
	ID        int64              `json:"id"`
	Numero    int64              `json:"numero"`
	Status    string             `json:"status"`
	CriadoEm  time.Time          `json:"criadoEm"`
	FechadoEm *time.Time         `json:"fechadoEm,omitempty"`
	Itens     []itemRespostaJSON `json:"itens"`
}

func notaParaJSON(n domain.NotaFiscal) notaRespostaJSON {
	itens := make([]itemRespostaJSON, 0, len(n.Itens))
	for _, it := range n.Itens {
		itens = append(itens, itemRespostaJSON{
			ProdutoCodigo:    it.ProdutoCodigo,
			ProdutoDescricao: it.ProdutoDescricao,
			Quantidade:       it.Quantidade,
		})
	}
	return notaRespostaJSON{
		ID:        n.ID,
		Numero:    n.Numero,
		Status:    string(n.Status),
		CriadoEm:  n.CriadoEm,
		FechadoEm: n.FechadoEm,
		Itens:     itens,
	}
}

// Criar trata POST /notas.
func (h *NotaHandler) Criar(w http.ResponseWriter, r *http.Request) {
	var req criarNotaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.EscreverValidacao(w, "REQUISICAO_INVALIDA", "Corpo da requisição inválido: esperado JSON com a lista de itens.")
		return
	}

	if len(req.Itens) == 0 {
		apierror.EscreverValidacao(w, "ITENS_OBRIGATORIOS", "A nota precisa de ao menos um item.")
		return
	}

	entradas := make([]domain.ItemEntrada, 0, len(req.Itens))
	for _, it := range req.Itens {
		if strings.TrimSpace(it.ProdutoCodigo) == "" || it.Quantidade <= 0 {
			apierror.EscreverValidacao(w, "ITEM_INVALIDO", "Cada item precisa de produtoCodigo e quantidade positiva.")
			return
		}
		entradas = append(entradas, domain.ItemEntrada{ProdutoCodigo: it.ProdutoCodigo, Quantidade: it.Quantidade})
	}

	nota, err := h.servico.Criar(r.Context(), entradas)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(notaParaJSON(nota))
}

// Listar trata GET /notas.
func (h *NotaHandler) Listar(w http.ResponseWriter, r *http.Request) {
	notas, err := h.servico.Listar(r.Context())
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	resposta := make([]notaRespostaJSON, 0, len(notas))
	for _, n := range notas {
		resposta = append(resposta, notaParaJSON(n))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resposta)
}

// BuscarPorID trata GET /notas/{id}.
func (h *NotaHandler) BuscarPorID(w http.ResponseWriter, r *http.Request) {
	id, err := idDaRota(r)
	if err != nil {
		apierror.EscreverValidacao(w, "ID_INVALIDO", "O ID da nota deve ser um número inteiro.")
		return
	}

	nota, err := h.servico.BuscarPorID(r.Context(), id)
	if err != nil {
		apierror.EscreverErro(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(notaParaJSON(nota))
}

// idDaRota extrai o {id} de rotas registradas como "/notas/{id}" e
// "/notas/{id}/imprimir" usando r.PathValue (net/http, Go 1.22+).
func idDaRota(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
