package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/repository"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/service"
)

func criarNotaDeTeste(t *testing.T, repo *repository.NotaRepository) domain.NotaFiscal {
	t.Helper()
	nota, err := repo.Criar(context.Background(), []domain.ItemNota{
		{ProdutoCodigo: "PARAF-001", ProdutoDescricao: "Parafuso sextavado M6", Quantidade: 2},
	})
	if err != nil {
		t.Fatalf("criar nota de teste: %v", err)
	}
	return nota
}

func TestImprimirService_CaminhoFeliz(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	nota := criarNotaDeTeste(t, repo)

	var chamadasBaixa int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/produtos/baixa" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&chamadasBaixa, 1)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(estoqueclient.RespostaBaixa{
			Itens: []estoqueclient.ItemBaixaResultado{{Codigo: "PARAF-001", SaldoAnterior: 10, SaldoAtual: 8}},
		})
	}))
	defer servidor.Close()

	svc := service.NovoImprimirService(repo, estoqueclient.NovoCliente(servidor.URL))
	resultado, err := svc.Imprimir(context.Background(), nota.ID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resultado.Status != domain.StatusFechada {
		t.Fatalf("esperava status Fechada, obteve %s", resultado.Status)
	}
	if resultado.FechadoEm == nil {
		t.Fatal("esperava fechado_em preenchido")
	}
	if atomic.LoadInt32(&chamadasBaixa) != 1 {
		t.Fatalf("esperava exatamente 1 chamada de baixa, obteve %d", chamadasBaixa)
	}
}

func TestImprimirService_NotaJaFechada_NaoChamaEstoque(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	nota := criarNotaDeTeste(t, repo)

	var chamadasBaixa int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadasBaixa, 1)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(estoqueclient.RespostaBaixa{})
	}))
	defer servidor.Close()

	svc := service.NovoImprimirService(repo, estoqueclient.NovoCliente(servidor.URL))

	// primeira impressão fecha a nota
	if _, err := svc.Imprimir(context.Background(), nota.ID); err != nil {
		t.Fatalf("primeira impressão falhou: %v", err)
	}

	// segunda impressão: nota já fechada, deve ser rejeitada com 409 sem
	// bater no estoque de novo
	chamadasAntes := atomic.LoadInt32(&chamadasBaixa)
	_, err := svc.Imprimir(context.Background(), nota.ID)
	if err != domain.ErrNotaNaoAberta {
		t.Fatalf("esperava ErrNotaNaoAberta, obteve %v", err)
	}
	if atomic.LoadInt32(&chamadasBaixa) != chamadasAntes {
		t.Fatalf("nota já fechada não deveria chamar o estoque de novo: antes=%d depois=%d", chamadasAntes, chamadasBaixa)
	}
}

func TestImprimirService_EstoqueRespondeErroDeNegocio_NotaPermaneceAberta(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	nota := criarNotaDeTeste(t, repo)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"erro": map[string]any{
				"codigo": "SALDO_INSUFICIENTE", "mensagem": "sem saldo suficiente", "tipo": "negocio", "repetivel": false,
			},
		})
	}))
	defer servidor.Close()

	svc := service.NovoImprimirService(repo, estoqueclient.NovoCliente(servidor.URL))
	_, err := svc.Imprimir(context.Background(), nota.ID)

	var erroNegocio *domain.ErroNegocioEstoque
	if e, ok := err.(*domain.ErroNegocioEstoque); ok {
		erroNegocio = e
	} else {
		t.Fatalf("esperava *domain.ErroNegocioEstoque, obteve %T: %v", err, err)
	}
	if erroNegocio.Codigo != "SALDO_INSUFICIENTE" {
		t.Fatalf("código não propagado corretamente: %s", erroNegocio.Codigo)
	}

	notaAtual, err := repo.BuscarPorID(context.Background(), nota.ID)
	if err != nil {
		t.Fatalf("buscar nota: %v", err)
	}
	if notaAtual.Status != domain.StatusAberta {
		t.Fatalf("nota deveria permanecer Aberta após erro de negócio, está %s", notaAtual.Status)
	}
}

func TestImprimirService_EstoqueIndisponivel_EsgotaRetriesENotaPermaneceAberta(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	nota := criarNotaDeTeste(t, repo)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer servidor.Close()

	svc := service.NovoImprimirService(repo, estoqueclient.NovoCliente(servidor.URL))
	_, err := svc.Imprimir(context.Background(), nota.ID)
	if err != domain.ErrEstoqueIndisponivel {
		t.Fatalf("esperava ErrEstoqueIndisponivel, obteve %v", err)
	}

	notaAtual, err := repo.BuscarPorID(context.Background(), nota.ID)
	if err != nil {
		t.Fatalf("buscar nota: %v", err)
	}
	if notaAtual.Status != domain.StatusAberta {
		t.Fatalf("nota deveria permanecer Aberta após esgotar tentativas, está %s", notaAtual.Status)
	}
}

func TestImprimirService_ReimpressaoAposFalhaReusaMesmaChaveDeIdempotencia(t *testing.T) {
	pool := poolDeTeste(t)
	repo := repository.NovoNotaRepository(pool)
	nota := criarNotaDeTeste(t, repo)

	var mu sync.Mutex
	var chavesRecebidas []string
	var totalChamadas int

	// as 3 primeiras chamadas HTTP (1 original + 2 retries da primeira
	// tentativa de impressão) falham, esgotando as tentativas da primeira
	// chamada a Imprimir; a partir da 4ª chamada (já na reimpressão) o
	// estoque "volta" e responde com sucesso.
	const chamadasQueFalham = 3

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		totalChamadas++
		chavesRecebidas = append(chavesRecebidas, r.Header.Get("Idempotency-Key"))
		deveFalhar := totalChamadas <= chamadasQueFalham
		mu.Unlock()

		if deveFalhar {
			// simula o cenário do plano técnico: o estoque está
			// indisponível durante a primeira tentativa de impressão.
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(estoqueclient.RespostaBaixa{
			Itens: []estoqueclient.ItemBaixaResultado{{Codigo: "PARAF-001", SaldoAnterior: 10, SaldoAtual: 8}},
		})
	}))
	defer servidor.Close()

	svc := service.NovoImprimirService(repo, estoqueclient.NovoCliente(servidor.URL))

	// primeira tentativa de impressão: estoque indisponível, nota continua Aberta
	_, err := svc.Imprimir(context.Background(), nota.ID)
	if err != domain.ErrEstoqueIndisponivel {
		t.Fatalf("esperava ErrEstoqueIndisponivel na primeira tentativa, obteve %v", err)
	}

	// segunda tentativa (reimpressão): deve suceder e fechar a nota
	resultado, err := svc.Imprimir(context.Background(), nota.ID)
	if err != nil {
		t.Fatalf("esperava sucesso na reimpressão, obteve erro: %v", err)
	}
	if resultado.Status != domain.StatusFechada {
		t.Fatalf("esperava nota Fechada após reimpressão, obteve %s", resultado.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(chavesRecebidas) < 2 {
		t.Fatalf("esperava ao menos 2 chamadas ao estoque, obteve %d", len(chavesRecebidas))
	}
	chaveEsperada := service.ChaveIdempotencia(nota.ID)
	for _, chave := range chavesRecebidas {
		if chave != chaveEsperada {
			t.Fatalf("esperava sempre a mesma chave de idempotência %q, obteve %q em alguma chamada", chaveEsperada, chave)
		}
	}
}
