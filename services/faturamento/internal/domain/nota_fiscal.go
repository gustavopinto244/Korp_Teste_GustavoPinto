// Package domain contém as entidades e regras de negócio puras do serviço de
// faturamento. Este pacote não importa net/http nem nenhum driver de banco de
// dados — camadas superiores (service, repository, handler) dependem dele,
// nunca o contrário.
package domain

import "time"

// StatusNota representa o ciclo de vida de uma nota fiscal. O único destino
// possível a partir de Aberta é Fechada — não há caminho de volta.
type StatusNota string

const (
	StatusAberta  StatusNota = "Aberta"
	StatusFechada StatusNota = "Fechada"
)

// ItemNota é uma linha da nota fiscal: referência lógica a um produto do
// estoque (sem FK entre schemas) mais a descrição capturada no momento da
// criação (snapshot) e a quantidade solicitada.
type ItemNota struct {
	ID               int64
	NotaID           int64
	ProdutoCodigo    string
	ProdutoDescricao string
	Quantidade       int
}

// NotaFiscal é a entidade raiz do serviço de faturamento. Numero é gerado
// pelo banco via SEQUENCE — nunca calculado em memória — para sobreviver a
// requisições concorrentes sem duplicar numeração.
type NotaFiscal struct {
	ID        int64
	Numero    int64
	Status    StatusNota
	CriadoEm  time.Time
	FechadoEm *time.Time
	Itens     []ItemNota
}

// EstaAberta indica se a nota ainda pode ser impressa.
func (n NotaFiscal) EstaAberta() bool {
	return n.Status == StatusAberta
}

// ItemEntrada representa um item informado na criação de uma nota, antes de
// ser validado contra o catálogo de produtos do estoque.
type ItemEntrada struct {
	ProdutoCodigo string
	Quantidade    int
}
