package service_test

// Testes do opcional de tratamento de concorrência: várias baixas
// simultâneas sobre os mesmos produtos. Todos rodam contra Postgres real —
// concorrência de banco não se prova com mock — e por isso são pulados
// quando TEST_DATABASE_URL_ESTOQUE não está configurada.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/estoque/internal/service"
)

// dispararEmParalelo executa fn quantidade vezes simultaneamente, com todas
// as goroutines liberadas ao mesmo tempo, e devolve os erros na ordem do
// índice.
func dispararEmParalelo(quantidade int, fn func(i int) error) []error {
	erros := make([]error, quantidade)
	largada := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < quantidade; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-largada
			erros[i] = fn(i)
		}(i)
	}

	close(largada)
	wg.Wait()
	return erros
}

// TestConcorrencia_SaldoDisputadoNaoFicaNegativo é o caso clássico: saldo 1
// disputado por 8 requisições simultâneas. Exatamente uma pode vencer; as
// outras precisam receber saldo insuficiente, nunca um saldo negativo no
// banco. Quem garante isso é o SELECT ... FOR UPDATE do repositório, que
// serializa a leitura-e-débito de cada produto.
func TestConcorrencia_SaldoDisputadoNaoFicaNegativo(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "CONC-001", "Parafuso disputado", 1); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	const concorrentes = 8
	erros := dispararEmParalelo(concorrentes, func(i int) error {
		_, err := baixaSvc.Processar(ctx, fmt.Sprintf("chave-disputa-%d", i), service.RequisicaoBaixa{
			Itens: []service.ItemBaixa{{Codigo: "CONC-001", Quantidade: 1}},
		})
		return err
	})

	var sucessos int
	for i, err := range erros {
		switch {
		case err == nil:
			sucessos++
		case ehSaldoInsuficiente(err):
			// esperado para os perdedores da disputa
		default:
			t.Errorf("requisição %d falhou por motivo inesperado: %v", i, err)
		}
	}

	if sucessos != 1 {
		t.Fatalf("esperava exatamente 1 baixa bem-sucedida, obteve %d", sucessos)
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "CONC-001")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 0 {
		t.Fatalf("saldo final = %d, esperado 0", produto.Saldo)
	}
}

// TestConcorrencia_SemLostUpdate cobre o outro lado: com saldo suficiente
// para todos, cada baixa precisa ser contabilizada. Se duas transações
// lessem o mesmo saldo antes de qualquer débito, uma sobrescreveria a outra
// e o saldo final ficaria alto demais — o lost update clássico.
func TestConcorrencia_SemLostUpdate(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	const saldoInicial, concorrentes = 20, 10
	if _, err := produtoRepo.Criar(ctx, "CONC-002", "Martelo", saldoInicial); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	erros := dispararEmParalelo(concorrentes, func(i int) error {
		_, err := baixaSvc.Processar(ctx, fmt.Sprintf("chave-lost-%d", i), service.RequisicaoBaixa{
			Itens: []service.ItemBaixa{{Codigo: "CONC-002", Quantidade: 1}},
		})
		return err
	})

	for i, err := range erros {
		if err != nil {
			t.Fatalf("requisição %d deveria caber no saldo, mas falhou: %v", i, err)
		}
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "CONC-002")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if esperado := saldoInicial - concorrentes; produto.Saldo != esperado {
		t.Fatalf("saldo final = %d, esperado %d (lost update)", produto.Saldo, esperado)
	}
}

// TestConcorrencia_MesmaChaveSimultanea é o duplo clique em Imprimir: duas
// requisições idênticas partem juntas, antes de a primeira ter commitado.
// Nenhuma pode falhar com erro de infraestrutura e o saldo só pode cair uma
// vez — é a idempotência valendo também sob concorrência, não apenas em
// chamadas sequenciais.
func TestConcorrencia_MesmaChaveSimultanea(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	if _, err := produtoRepo.Criar(ctx, "CONC-003", "Disco de corte", 10); err != nil {
		t.Fatalf("criar produto: %v", err)
	}

	const concorrentes = 6
	requisicao := service.RequisicaoBaixa{
		Itens: []service.ItemBaixa{{Codigo: "CONC-003", Quantidade: 2}},
	}

	respostas := make([]*service.RespostaBaixa, concorrentes)
	erros := dispararEmParalelo(concorrentes, func(i int) error {
		resp, err := baixaSvc.Processar(ctx, "impressao-nota-42", requisicao)
		respostas[i] = resp
		return err
	})

	for i, err := range erros {
		if err != nil {
			t.Fatalf("requisição %d falhou; a chave repetida deveria dar replay: %v", i, err)
		}
		if respostas[i] == nil || len(respostas[i].Itens) != 1 {
			t.Fatalf("requisição %d devolveu resposta inesperada: %+v", i, respostas[i])
		}
		if got := respostas[i].Itens[0]; got.SaldoAnterior != 10 || got.SaldoAtual != 8 {
			t.Fatalf("requisição %d devolveu %+v, esperado saldo 10 -> 8 em todas", i, got)
		}
	}

	produto, err := produtoRepo.BuscarPorCodigo(ctx, "CONC-003")
	if err != nil {
		t.Fatalf("buscar produto: %v", err)
	}
	if produto.Saldo != 8 {
		t.Fatalf("saldo final = %d, esperado 8 (débito único apesar de %d chamadas)", produto.Saldo, concorrentes)
	}
}

// TestConcorrencia_OrdemInvertidaNaoTravaEmDeadlock cobre o deadlock que
// duas notas com os mesmos produtos em ordem oposta causariam se cada
// transação travasse as linhas na ordem em que o cliente as enviou. O
// serviço adquire os locks sempre em ordem de código, então as duas
// transações disputam as mesmas linhas na mesma sequência e uma apenas
// espera a outra.
func TestConcorrencia_OrdemInvertidaNaoTravaEmDeadlock(t *testing.T) {
	baixaSvc, produtoRepo := setup(t)
	ctx := context.Background()

	for _, p := range []struct {
		codigo    string
		descricao string
	}{
		{"CONC-A", "Produto A"},
		{"CONC-B", "Produto B"},
	} {
		if _, err := produtoRepo.Criar(ctx, p.codigo, p.descricao, 100); err != nil {
			t.Fatalf("criar produto %s: %v", p.codigo, err)
		}
	}

	direta := []service.ItemBaixa{{Codigo: "CONC-A", Quantidade: 1}, {Codigo: "CONC-B", Quantidade: 1}}
	invertida := []service.ItemBaixa{{Codigo: "CONC-B", Quantidade: 1}, {Codigo: "CONC-A", Quantidade: 1}}

	const pares = 12
	erros := dispararEmParalelo(pares, func(i int) error {
		itens := direta
		if i%2 == 1 {
			itens = invertida
		}
		_, err := baixaSvc.Processar(ctx, fmt.Sprintf("chave-ordem-%d", i), service.RequisicaoBaixa{Itens: itens})
		return err
	})

	for i, err := range erros {
		if err != nil {
			t.Fatalf("requisição %d falhou (provável deadlock): %v", i, err)
		}
	}

	for _, codigo := range []string{"CONC-A", "CONC-B"} {
		produto, err := produtoRepo.BuscarPorCodigo(ctx, codigo)
		if err != nil {
			t.Fatalf("buscar produto %s: %v", codigo, err)
		}
		if esperado := 100 - pares; produto.Saldo != esperado {
			t.Fatalf("saldo de %s = %d, esperado %d", codigo, produto.Saldo, esperado)
		}
	}
}

func ehSaldoInsuficiente(err error) bool {
	var insuficiente *domain.ErrSaldoInsuficiente
	return errors.As(err, &insuficiente)
}
