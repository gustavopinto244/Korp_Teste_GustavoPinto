package apierror_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/apierror"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
)

func TestEscreverErro(t *testing.T) {
	casos := []struct {
		nome           string
		err            error
		statusEsperado int
		codigoEsperado string
		tipoEsperado   string
		repetivelEsper bool
		// mensagemEsperada, quando preenchida, exige a mensagem literal —
		// usado nos códigos cujo texto é parte do contrato compartilhado
		// com o serviço de faturamento.
		mensagemEsperada string
	}{
		{
			nome:           "produto não encontrado",
			err:            domain.ErrProdutoNaoEncontrado,
			statusEsperado: 404,
			codigoEsperado: "PRODUTO_NAO_ENCONTRADO",
			tipoEsperado:   apierror.TipoNegocio,
			repetivelEsper: false,
		},
		{
			nome:           "código duplicado",
			err:            domain.ErrCodigoDuplicado,
			statusEsperado: 409,
			codigoEsperado: "CODIGO_DUPLICADO",
			tipoEsperado:   apierror.TipoNegocio,
			repetivelEsper: false,
		},
		{
			nome:           "validação",
			err:            domain.ErrValidacao,
			statusEsperado: 422,
			codigoEsperado: "VALIDACAO",
			tipoEsperado:   apierror.TipoNegocio,
			repetivelEsper: false,
		},
		{
			nome: "saldo insuficiente",
			err: &domain.ErrSaldoInsuficiente{
				Codigo:     "PARAF-001",
				Disponivel: 3,
				Solicitado: 5,
			},
			statusEsperado: 422,
			codigoEsperado: "SALDO_INSUFICIENTE",
			tipoEsperado:   apierror.TipoNegocio,
			repetivelEsper: false,
		},
		{
			nome:           "chave de idempotência conflitante",
			err:            domain.ErrChaveIdempotenciaConflitante,
			statusEsperado: 409,
			codigoEsperado: "IDEMPOTENCY_KEY_CONFLITO",
			tipoEsperado:   apierror.TipoSistema,
			repetivelEsper: false,
		},
		{
			nome:           "chave de idempotência ausente",
			err:            domain.ErrChaveIdempotenciaAusente,
			statusEsperado: 400,
			codigoEsperado: "IDEMPOTENCY_KEY_AUSENTE",
			tipoEsperado:   apierror.TipoNegocio,
			repetivelEsper: false,
		},
		{
			// Formato canônico de ERRO_INTERNO, comum aos dois
			// microsserviços e consumido pelo frontend. Qualquer alteração
			// aqui precisa ser espelhada no serviço de faturamento.
			nome:             "erro inesperado",
			err:              errors.New("boom"),
			statusEsperado:   500,
			codigoEsperado:   "ERRO_INTERNO",
			tipoEsperado:     apierror.TipoSistema,
			repetivelEsper:   true,
			mensagemEsperada: "Ocorreu um erro inesperado. Tente novamente em instantes.",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rec := httptest.NewRecorder()
			apierror.EscreverErro(rec, c.err)

			if rec.Code != c.statusEsperado {
				t.Errorf("status = %d, esperado %d", rec.Code, c.statusEsperado)
			}

			var envelope apierror.EnvelopeErro
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("resposta não é JSON válido: %v", err)
			}

			if envelope.Erro.Codigo != c.codigoEsperado {
				t.Errorf("codigo = %q, esperado %q", envelope.Erro.Codigo, c.codigoEsperado)
			}
			if envelope.Erro.Tipo != c.tipoEsperado {
				t.Errorf("tipo = %q, esperado %q", envelope.Erro.Tipo, c.tipoEsperado)
			}
			if envelope.Erro.Repetivel != c.repetivelEsper {
				t.Errorf("repetivel = %v, esperado %v", envelope.Erro.Repetivel, c.repetivelEsper)
			}
			if envelope.Erro.Mensagem == "" {
				t.Errorf("mensagem vazia")
			}
			if c.mensagemEsperada != "" && envelope.Erro.Mensagem != c.mensagemEsperada {
				t.Errorf("mensagem = %q, esperada %q", envelope.Erro.Mensagem, c.mensagemEsperada)
			}
		})
	}
}
