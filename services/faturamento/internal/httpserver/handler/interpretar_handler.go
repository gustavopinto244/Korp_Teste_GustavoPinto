package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/ia"
)

// CatalogoFornecedor é o subconjunto do cliente de estoque necessário para
// obter o catálogo real de produtos antes de interpretar o texto livre.
type CatalogoFornecedor interface {
	ListarProdutos(ctx context.Context) ([]estoqueclient.Produto, error)
}

// InterpretarHandler trata POST /notas/interpretar — suporte opcional de IA
// à criação manual de nota, nunca persiste nada e nunca derruba o restante
// do sistema em caso de falha (degrada para 503 IA_INDISPONIVEL).
type InterpretarHandler struct {
	catalogo      CatalogoFornecedor
	interpretador ia.InterpretadorDeTexto
	timeout       time.Duration
}

// NovoInterpretarHandler constrói um InterpretarHandler. O timeout cobre a
// consulta ao catálogo e a interpretação inteira; ele varia com o provider
// de IA configurado, por isso vem de fora (ver cmd/api/main.go).
func NovoInterpretarHandler(catalogo CatalogoFornecedor, interpretador ia.InterpretadorDeTexto, timeout time.Duration) *InterpretarHandler {
	return &InterpretarHandler{catalogo: catalogo, interpretador: interpretador, timeout: timeout}
}

type interpretarRequest struct {
	Texto string `json:"texto"`
}

// Interpretar trata POST /notas/interpretar.
func (h *InterpretarHandler) Interpretar(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	var req interpretarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Texto) == "" {
		apierror.EscreverValidacao(w, "TEXTO_OBRIGATORIO", "Informe o campo texto com a descrição livre dos itens.")
		return
	}

	produtos, err := h.catalogo.ListarProdutos(ctx)
	if err != nil {
		apierror.EscreverErro(w, domain.ErrIAIndisponivel)
		return
	}

	catalogo := make([]ia.CatalogoItem, 0, len(produtos))
	for _, p := range produtos {
		catalogo = append(catalogo, ia.CatalogoItem{
			Codigo:          p.Codigo,
			Descricao:       p.Descricao,
			SaldoDisponivel: p.Saldo,
		})
	}

	resultado, err := h.interpretador.Interpretar(ctx, req.Texto, catalogo)
	if err != nil {
		// Nunca deixa a falha do interpretador derrubar o restante do
		// sistema — a IA é só apoio opcional à criação manual de nota. O
		// usuário recebe 503 e continua cadastrando os itens à mão; o
		// motivo real fica no log do serviço, já que a resposta não o
		// carrega.
		log.Printf("faturamento: interpretação por IA indisponível: %v", err)
		apierror.EscreverErro(w, domain.ErrIAIndisponivel)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resultado)
}
