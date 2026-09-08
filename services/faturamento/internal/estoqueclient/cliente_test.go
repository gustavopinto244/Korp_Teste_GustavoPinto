package estoqueclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

func TestBaixarEstoque_SucessoNaPrimeiraTentativa(t *testing.T) {
	var chamadas int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		if r.Header.Get("Idempotency-Key") != "impressao-nota-1" {
			t.Errorf("esperava header Idempotency-Key impressao-nota-1, obteve %q", r.Header.Get("Idempotency-Key"))
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(RespostaBaixa{Itens: []ItemBaixaResultado{{Codigo: "PARAF-001", SaldoAnterior: 10, SaldoAtual: 8}}})
	}))
	defer servidor.Close()

	c := NovoCliente(servidor.URL)
	resp, err := c.BaixarEstoque(context.Background(), "impressao-nota-1", []ItemBaixa{{Codigo: "PARAF-001", Quantidade: 2}})
	if err != nil {
		t.Fatalf("esperava sucesso, obteve erro: %v", err)
	}
	if len(resp.Itens) != 1 || resp.Itens[0].SaldoAtual != 8 {
		t.Fatalf("resposta inesperada: %+v", resp)
	}
	if atomic.LoadInt32(&chamadas) != 1 {
		t.Fatalf("esperava exatamente 1 chamada, obteve %d", chamadas)
	}
}

func TestBaixarEstoque_NaoRepeteEmErroDeNegocio(t *testing.T) {
	var chamadas int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"erro": map[string]any{
				"codigo": "SALDO_INSUFICIENTE", "mensagem": "sem saldo", "tipo": "negocio", "repetivel": false,
			},
		})
	}))
	defer servidor.Close()

	c := NovoCliente(servidor.URL)
	_, err := c.BaixarEstoque(context.Background(), "impressao-nota-2", []ItemBaixa{{Codigo: "PARAF-001", Quantidade: 2}})
	if err == nil {
		t.Fatal("esperava erro")
	}
	var erroNegocio *domain.ErroNegocioEstoque
	if !asErroNegocio(err, &erroNegocio) {
		t.Fatalf("esperava *domain.ErroNegocioEstoque, obteve %T: %v", err, err)
	}
	if erroNegocio.Codigo != "SALDO_INSUFICIENTE" {
		t.Fatalf("código inesperado: %s", erroNegocio.Codigo)
	}
	if atomic.LoadInt32(&chamadas) != 1 {
		t.Fatalf("erro de negócio não deveria ser repetido: %d chamadas", chamadas)
	}
}

func TestBaixarEstoque_RepeteEm503EDepoisConsegue(t *testing.T) {
	var chamadas int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&chamadas, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(RespostaBaixa{Itens: []ItemBaixaResultado{{Codigo: "PARAF-001", SaldoAnterior: 10, SaldoAtual: 8}}})
	}))
	defer servidor.Close()

	c := NovoCliente(servidor.URL)
	resp, err := c.BaixarEstoque(context.Background(), "impressao-nota-3", []ItemBaixa{{Codigo: "PARAF-001", Quantidade: 2}})
	if err != nil {
		t.Fatalf("esperava sucesso após retries, obteve erro: %v", err)
	}
	if resp.Itens[0].SaldoAtual != 8 {
		t.Fatalf("resposta inesperada: %+v", resp)
	}
	if atomic.LoadInt32(&chamadas) != 3 {
		t.Fatalf("esperava 3 chamadas (1 original + 2 retries), obteve %d", chamadas)
	}
}

func TestBaixarEstoque_EsgotaTentativasEDevolveIndisponivel(t *testing.T) {
	var chamadas int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer servidor.Close()

	c := NovoCliente(servidor.URL)
	_, err := c.BaixarEstoque(context.Background(), "impressao-nota-4", []ItemBaixa{{Codigo: "PARAF-001", Quantidade: 2}})
	if err != domain.ErrEstoqueIndisponivel {
		t.Fatalf("esperava ErrEstoqueIndisponivel, obteve %v", err)
	}
	if atomic.LoadInt32(&chamadas) != int32(tentativasMaximas) {
		t.Fatalf("esperava %d tentativas, obteve %d", tentativasMaximas, chamadas)
	}
}

func TestCircuitBreaker_AbreApos5FalhasERecuperaDepoisDoIntervalo(t *testing.T) {
	var chamadas int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer servidor.Close()

	c := NovoCliente(servidor.URL)
	// reduz o intervalo de circuito aberto para o teste não demorar 10s
	c.breaker = novoCircuitBreaker(5, 50*time.Millisecond)

	// cada chamada a BaixarEstoque já consome até 3 tentativas; disparamos
	// chamadas até estourar o limite de 5 falhas consecutivas do breaker.
	for i := 0; i < 2; i++ {
		_, err := c.BaixarEstoque(context.Background(), "impressao-nota-breaker", []ItemBaixa{{Codigo: "X", Quantidade: 1}})
		if err != domain.ErrEstoqueIndisponivel {
			t.Fatalf("esperava ErrEstoqueIndisponivel, obteve %v", err)
		}
	}

	chamadasAntes := atomic.LoadInt32(&chamadas)
	// o circuito deve estar aberto agora (>=5 falhas): a próxima chamada não
	// deve nem bater no servidor.
	_, err := c.BaixarEstoque(context.Background(), "impressao-nota-breaker", []ItemBaixa{{Codigo: "X", Quantidade: 1}})
	if err != domain.ErrEstoqueIndisponivel {
		t.Fatalf("esperava ErrEstoqueIndisponivel com circuito aberto, obteve %v", err)
	}
	if atomic.LoadInt32(&chamadas) != chamadasAntes {
		t.Fatalf("circuito aberto deveria recusar chamada sem bater no servidor: antes=%d depois=%d", chamadasAntes, chamadas)
	}

	// espera o intervalo de circuito aberto passar: o breaker deve voltar a
	// permitir uma chamada de teste (half-open).
	time.Sleep(70 * time.Millisecond)
	if !c.breaker.permiteChamada() {
		t.Fatal("esperava que o breaker permitisse uma chamada de teste half-open após o intervalo")
	}
	c.breaker.registrarSucesso()
	if !c.breaker.permiteChamada() {
		t.Fatal("esperava circuito fechado após sucesso na chamada de teste")
	}
}

// asErroNegocio é um helper simples de type assertion para manter o teste
// legível sem importar "errors" só para isso.
func asErroNegocio(err error, target **domain.ErroNegocioEstoque) bool {
	e, ok := err.(*domain.ErroNegocioEstoque)
	if !ok {
		return false
	}
	*target = e
	return true
}
