package apierror_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
)

func decodificar(t *testing.T, rr *httptest.ResponseRecorder) apierror.Envelope {
	t.Helper()
	var env apierror.Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decodificar corpo de erro: %v", err)
	}
	return env
}

// TestEscreverErro_ErroInternoSegueFormatoCanonico trava o formato de
// ERRO_INTERNO no mesmo contrato usado pelo serviço de estoque: o frontend
// consome um formato só e o mesmo `codigo` não pode chegar com `repetivel`
// invertido conforme o serviço de origem.
func TestEscreverErro_ErroInternoSegueFormatoCanonico(t *testing.T) {
	rr := httptest.NewRecorder()
	apierror.EscreverErro(rr, errors.New("falha inesperada de infraestrutura"))

	if rr.Code != 500 {
		t.Fatalf("esperava status 500, obteve %d", rr.Code)
	}
	env := decodificar(t, rr)
	if env.Erro.Codigo != "ERRO_INTERNO" {
		t.Fatalf("código inesperado: %s", env.Erro.Codigo)
	}
	if env.Erro.Tipo != apierror.TipoSistema {
		t.Fatalf("tipo inesperado: %s", env.Erro.Tipo)
	}
	if !env.Erro.Repetivel {
		t.Fatal("ERRO_INTERNO deve ser repetível, como no serviço de estoque")
	}
	if env.Erro.Mensagem != "Ocorreu um erro inesperado. Tente novamente em instantes." {
		t.Fatalf("mensagem divergente do formato canônico: %q", env.Erro.Mensagem)
	}
}

// TestEscreverErro_ErroInternoRegistraErroOriginal garante que o erro
// original não é descartado silenciosamente — sem log, um 500 é indebugável,
// já que o corpo da resposta nunca carrega o erro real.
func TestEscreverErro_ErroInternoRegistraErroOriginal(t *testing.T) {
	var saida bytes.Buffer
	flagsOriginais := log.Flags()
	log.SetOutput(&saida)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flagsOriginais)
	})

	apierror.EscreverErro(httptest.NewRecorder(), errors.New("conexão com o banco recusada"))

	if !strings.Contains(saida.String(), "conexão com o banco recusada") {
		t.Fatalf("esperava o erro original no log, obtive: %q", saida.String())
	}
}

// TestEscreverErro_ErroConhecidoNaoPoluiLog confirma que só o 500 vai para o
// log: erros de negócio esperados não são ruído operacional.
func TestEscreverErro_ErroConhecidoNaoPoluiLog(t *testing.T) {
	var saida bytes.Buffer
	flagsOriginais := log.Flags()
	log.SetOutput(&saida)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flagsOriginais)
	})

	apierror.EscreverErro(httptest.NewRecorder(), domain.ErrNotaNaoAberta)

	if saida.Len() != 0 {
		t.Fatalf("erro de negócio não deveria ser logado, obtive: %q", saida.String())
	}
}

func TestEscreverErro_NotaNaoAberta(t *testing.T) {
	rr := httptest.NewRecorder()
	apierror.EscreverErro(rr, domain.ErrNotaNaoAberta)

	if rr.Code != 409 {
		t.Fatalf("esperava status 409, obteve %d", rr.Code)
	}
	env := decodificar(t, rr)
	if env.Erro.Codigo != "NOTA_NAO_ABERTA" {
		t.Fatalf("código inesperado: %s", env.Erro.Codigo)
	}
	if env.Erro.Repetivel {
		t.Fatal("NOTA_NAO_ABERTA não deveria ser repetível")
	}
}

func TestEscreverErro_EstoqueIndisponivel(t *testing.T) {
	rr := httptest.NewRecorder()
	apierror.EscreverErro(rr, domain.ErrEstoqueIndisponivel)

	if rr.Code != 503 {
		t.Fatalf("esperava status 503, obteve %d", rr.Code)
	}
	env := decodificar(t, rr)
	if env.Erro.Codigo != "ESTOQUE_INDISPONIVEL" {
		t.Fatalf("código inesperado: %s", env.Erro.Codigo)
	}
	if env.Erro.Tipo != apierror.TipoSistema {
		t.Fatalf("tipo inesperado: %s", env.Erro.Tipo)
	}
	if !env.Erro.Repetivel {
		t.Fatal("ESTOQUE_INDISPONIVEL deveria ser repetível")
	}
}

func TestEscreverErro_PropagaErroNegocioEstoque(t *testing.T) {
	casos := []struct {
		nome           string
		erro           *domain.ErroNegocioEstoque
		statusEsperado int
	}{
		{"saldo insuficiente", &domain.ErroNegocioEstoque{Codigo: "SALDO_INSUFICIENTE", Mensagem: "sem saldo", Tipo: apierror.TipoNegocio, Repetivel: false}, 422},
		{"produto não encontrado", &domain.ErroNegocioEstoque{Codigo: "PRODUTO_NAO_ENCONTRADO", Mensagem: "não existe", Tipo: apierror.TipoNegocio, Repetivel: false}, 404},
		{"conflito idempotência", &domain.ErroNegocioEstoque{Codigo: "IDEMPOTENCY_KEY_CONFLITO", Mensagem: "conflito", Tipo: apierror.TipoSistema, Repetivel: false}, 409},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rr := httptest.NewRecorder()
			apierror.EscreverErro(rr, c.erro)

			if rr.Code != c.statusEsperado {
				t.Fatalf("esperava status %d, obteve %d", c.statusEsperado, rr.Code)
			}
			env := decodificar(t, rr)
			if env.Erro.Codigo != c.erro.Codigo {
				t.Fatalf("código não propagado: esperava %s, obteve %s", c.erro.Codigo, env.Erro.Codigo)
			}
			if env.Erro.Mensagem != c.erro.Mensagem {
				t.Fatalf("mensagem não propagada")
			}
		})
	}
}
