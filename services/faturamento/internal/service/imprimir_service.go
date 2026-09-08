package service

import (
	"context"
	"fmt"

	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/domain"
	"github.com/gustavopinto244/korp-teste-gustavopinto/services/faturamento/internal/estoqueclient"
)

// ImprimirRepositorio é o subconjunto de internal/repository.NotaRepository
// necessário para o fluxo de impressão.
type ImprimirRepositorio interface {
	BuscarPorID(ctx context.Context, id int64) (domain.NotaFiscal, error)
	MarcarFechada(ctx context.Context, id int64) (domain.NotaFiscal, error)
}

// EstoqueBaixador é o subconjunto do cliente de estoque necessário para
// executar a baixa de itens durante a impressão.
type EstoqueBaixador interface {
	BaixarEstoque(ctx context.Context, idempotencyKey string, itens []estoqueclient.ItemBaixa) (estoqueclient.RespostaBaixa, error)
}

// ImprimirService orquestra o fluxo central do sistema: imprimir uma nota
// fiscal, o que dispara a baixa de estoque no serviço estoque e só então
// fecha a nota.
type ImprimirService struct {
	repo    ImprimirRepositorio
	estoque EstoqueBaixador
}

// NovoImprimirService constrói um ImprimirService.
func NovoImprimirService(repo ImprimirRepositorio, estoque EstoqueBaixador) *ImprimirService {
	return &ImprimirService{repo: repo, estoque: estoque}
}

// ChaveIdempotencia monta a chave de idempotência determinística usada em
// toda chamada de baixa de estoque para uma nota — a mesma chave é reusada
// em toda reimpressão da mesma nota, o que é o que permite ao estoque
// detectar replay e não debitar duas vezes.
func ChaveIdempotencia(notaID int64) string {
	return fmt.Sprintf("impressao-nota-%d", notaID)
}

// Imprimir executa o fluxo descrito na seção 3 do plano técnico:
//  1. busca a nota; se não estiver Aberta, 409 NOTA_NAO_ABERTA, sem chamar o estoque;
//  2. monta a chave de idempotência determinística;
//  3. chama o estoque para debitar os itens;
//  4. em sucesso, fecha a nota (segunda trava via WHERE status='Aberta');
//  5. em erro de negócio do estoque, propaga o erro e mantém a nota Aberta;
//  6. em esgotamento de tentativas/circuito aberto, devolve
//     domain.ErrEstoqueIndisponivel e mantém a nota Aberta.
func (s *ImprimirService) Imprimir(ctx context.Context, notaID int64) (domain.NotaFiscal, error) {
	nota, err := s.repo.BuscarPorID(ctx, notaID)
	if err != nil {
		return domain.NotaFiscal{}, err
	}

	if !nota.EstaAberta() {
		return domain.NotaFiscal{}, domain.ErrNotaNaoAberta
	}

	chave := ChaveIdempotencia(nota.ID)
	itens := make([]estoqueclient.ItemBaixa, 0, len(nota.Itens))
	for _, it := range nota.Itens {
		itens = append(itens, estoqueclient.ItemBaixa{Codigo: it.ProdutoCodigo, Quantidade: it.Quantidade})
	}

	if _, err := s.estoque.BaixarEstoque(ctx, chave, itens); err != nil {
		// erro de negócio (422/404/409) ou indisponibilidade
		// (domain.ErrEstoqueIndisponivel): em ambos os casos a nota
		// permanece Aberta, o erro sobe como está para o handler traduzir.
		return domain.NotaFiscal{}, err
	}

	return s.repo.MarcarFechada(ctx, nota.ID)
}
