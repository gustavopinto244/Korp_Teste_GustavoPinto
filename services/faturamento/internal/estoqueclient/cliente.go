// Package estoqueclient implementa o cliente HTTP resiliente que o serviço
// de faturamento usa para conversar com o serviço de estoque. Resiliência
// (retry com backoff+jitter e circuit breaker) é implementação própria, sem
// bibliotecas externas — decisão registrada no plano técnico.
package estoqueclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

const (
	timeoutPorTentativa  = 2 * time.Second
	tentativasMaximas    = 3
	limiteFalhasBreaker  = 5
	duracaoBreakerAberto = 10 * time.Second
)

// Produto é a representação do produto do estoque, conforme
// GET /produtos/{codigo} e GET /produtos.
type Produto struct {
	ID        int64  `json:"id"`
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	Saldo     int    `json:"saldo"`
}

// ItemBaixa é um item do corpo de POST /produtos/baixa.
type ItemBaixa struct {
	Codigo     string `json:"codigo"`
	Quantidade int    `json:"quantidade"`
}

// ItemBaixaResultado é um item da resposta de sucesso de POST /produtos/baixa.
type ItemBaixaResultado struct {
	Codigo        string `json:"codigo"`
	SaldoAnterior int    `json:"saldoAnterior"`
	SaldoAtual    int    `json:"saldoAtual"`
}

// RespostaBaixa é o corpo de sucesso de POST /produtos/baixa.
type RespostaBaixa struct {
	Itens []ItemBaixaResultado `json:"itens"`
}

type envelopeErro struct {
	Erro struct {
		Codigo    string `json:"codigo"`
		Mensagem  string `json:"mensagem"`
		Tipo      string `json:"tipo"`
		Repetivel bool   `json:"repetivel"`
	} `json:"erro"`
}

// Cliente é o cliente HTTP resiliente para o serviço de estoque.
type Cliente struct {
	baseURL    string
	httpClient *http.Client
	breaker    *circuitBreaker
}

// NovoCliente constrói um Cliente apontando para baseURL (ex.:
// "http://estoque:8081").
func NovoCliente(baseURL string) *Cliente {
	return &Cliente{
		baseURL:    baseURL,
		httpClient: &http.Client{
			// o timeout por tentativa é aplicado via context.WithTimeout em
			// cada chamada, não aqui — permite reaproveitar o mesmo
			// http.Client entre tentativas.
		},
		breaker: novoCircuitBreaker(limiteFalhasBreaker, duracaoBreakerAberto),
	}
}

// BuscarProduto chama GET /produtos/{codigo}. Devolve domain.ErrProdutoInvalido
// se o estoque responder 404, ou domain.ErrEstoqueIndisponivel se todas as
// tentativas se esgotarem por falha de rede/timeout ou o circuito estiver
// aberto.
func (c *Cliente) BuscarProduto(ctx context.Context, codigo string) (Produto, error) {
	var produto Produto
	status, corpo, err := c.executarComRetry(ctx, http.MethodGet, "/produtos/"+codigo, "", nil)
	if err != nil {
		return Produto{}, err
	}
	if status == http.StatusNotFound {
		return Produto{}, domain.ErrProdutoInvalido
	}
	if status != http.StatusOK {
		return Produto{}, erroNegocioDoCorpoOuGenerico(status, corpo)
	}
	if err := json.Unmarshal(corpo, &produto); err != nil {
		return Produto{}, fmt.Errorf("decodificar resposta de produto: %w", err)
	}
	return produto, nil
}

// ListarProdutos chama GET /produtos, usado pelo endpoint de interpretação
// por IA para obter o catálogo real.
func (c *Cliente) ListarProdutos(ctx context.Context) ([]Produto, error) {
	status, corpo, err := c.executarComRetry(ctx, http.MethodGet, "/produtos", "", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, erroNegocioDoCorpoOuGenerico(status, corpo)
	}
	var produtos []Produto
	if err := json.Unmarshal(corpo, &produtos); err != nil {
		return nil, fmt.Errorf("decodificar lista de produtos: %w", err)
	}
	return produtos, nil
}

// BaixarEstoque chama POST /produtos/baixa com a chave de idempotência
// informada. Em caso de sucesso (200), devolve o resultado. Em caso de erro
// de negócio (422/404/409), devolve *domain.ErroNegocioEstoque com os campos
// originais para propagação direta ao chamador do faturamento. Em caso de
// esgotamento de tentativas por indisponibilidade, devolve
// domain.ErrEstoqueIndisponivel.
func (c *Cliente) BaixarEstoque(ctx context.Context, idempotencyKey string, itens []ItemBaixa) (RespostaBaixa, error) {
	corpoReq, err := json.Marshal(struct {
		Itens []ItemBaixa `json:"itens"`
	}{Itens: itens})
	if err != nil {
		return RespostaBaixa{}, fmt.Errorf("serializar requisição de baixa: %w", err)
	}

	status, corpo, err := c.executarComRetry(ctx, http.MethodPost, "/produtos/baixa", idempotencyKey, corpoReq)
	if err != nil {
		return RespostaBaixa{}, err
	}

	if status != http.StatusOK {
		return RespostaBaixa{}, erroNegocioDoCorpoOuGenerico(status, corpo)
	}

	var resposta RespostaBaixa
	if err := json.Unmarshal(corpo, &resposta); err != nil {
		return RespostaBaixa{}, fmt.Errorf("decodificar resposta de baixa: %w", err)
	}
	return resposta, nil
}

// erroNegocioDoCorpoOuGenerico tenta decodificar o envelope de erro padrão
// do corpo de resposta; se não conseguir, monta um erro de negócio genérico
// a partir do status HTTP.
func erroNegocioDoCorpoOuGenerico(status int, corpo []byte) error {
	var env envelopeErro
	if err := json.Unmarshal(corpo, &env); err == nil && env.Erro.Codigo != "" {
		return &domain.ErroNegocioEstoque{
			Codigo:    env.Erro.Codigo,
			Mensagem:  env.Erro.Mensagem,
			Tipo:      env.Erro.Tipo,
			Repetivel: env.Erro.Repetivel,
		}
	}
	return &domain.ErroNegocioEstoque{
		Codigo:    "ERRO_ESTOQUE",
		Mensagem:  fmt.Sprintf("O serviço de estoque respondeu com status inesperado %d.", status),
		Tipo:      "sistema",
		Repetivel: false,
	}
}

// respostaRepetivel decide, a partir do status HTTP de uma resposta
// recebida com sucesso na camada de transporte, se vale a pena tentar de
// novo.
func respostaRepetivel(status int) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// executarComRetry executa uma requisição HTTP contra o estoque, aplicando
// timeout por tentativa, até tentativasMaximas tentativas apenas para
// respostas repetíveis (timeout, erro de conexão, 502/503/504), com backoff
// exponencial e jitter entre tentativas, e respeitando o circuit breaker.
// Devolve o status HTTP e o corpo da última resposta recebida com sucesso de
// transporte, ou domain.ErrEstoqueIndisponivel se as tentativas se
// esgotarem ou o circuito estiver aberto.
func (c *Cliente) executarComRetry(ctx context.Context, metodo, caminho, idempotencyKey string, corpoReq []byte) (int, []byte, error) {
	for tentativa := 1; tentativa <= tentativasMaximas; tentativa++ {
		if !c.breaker.permiteChamada() {
			return 0, nil, domain.ErrEstoqueIndisponivel
		}

		status, corpo, err := c.executarUmaVez(ctx, metodo, caminho, idempotencyKey, corpoReq)
		falhaRepetivel := err != nil || respostaRepetivel(status)

		if !falhaRepetivel {
			// sucesso de transporte (independente do status ser 2xx ou erro
			// de negócio não repetível): fecha o circuito e devolve para o
			// chamador decidir.
			c.breaker.registrarSucesso()
			return status, corpo, nil
		}

		// falha de rede/timeout ou 502/503/504: conta para o circuit
		// breaker e, se ainda houver tentativas, faz backoff e repete.
		c.breaker.registrarFalha()
		if tentativa < tentativasMaximas {
			if !aguardarBackoff(ctx, tentativa) {
				return 0, nil, domain.ErrEstoqueIndisponivel
			}
			continue
		}
	}

	return 0, nil, domain.ErrEstoqueIndisponivel
}

func aguardarBackoff(ctx context.Context, tentativa int) bool {
	select {
	case <-time.After(backoffDelay(tentativa)):
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Cliente) executarUmaVez(ctx context.Context, metodo, caminho, idempotencyKey string, corpoReq []byte) (int, []byte, error) {
	ctxTentativa, cancel := context.WithTimeout(ctx, timeoutPorTentativa)
	defer cancel()

	var reader io.Reader
	if corpoReq != nil {
		reader = bytes.NewReader(corpoReq)
	}

	req, err := http.NewRequestWithContext(ctxTentativa, metodo, c.baseURL+caminho, reader)
	if err != nil {
		return 0, nil, err
	}
	if corpoReq != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	corpo, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, corpo, nil
}
