package ia

import (
	"context"
	"errors"
)

// interpretadorClaude é o ponto de extensão visível para uma integração
// futura com a API da Claude (Anthropic). Nesta rodada é apenas um stub: não
// faz nenhuma chamada de rede, apenas sinaliza que a integração real ainda
// não foi implementada. O chamador (handler de /notas/interpretar) trata
// esse erro como IA indisponível e devolve 503, sem derrubar o restante do
// sistema.
type interpretadorClaude struct {
	apiKey string
}

// NovoInterpretadorClaude constrói o stub de interpretador via Claude. A
// apiKey é aceita para já fixar a assinatura que uma implementação real
// usaria, mas não é usada por este stub.
func NovoInterpretadorClaude(apiKey string) InterpretadorDeTexto {
	return &interpretadorClaude{apiKey: apiKey}
}

func (i *interpretadorClaude) Interpretar(ctx context.Context, texto string, catalogo []CatalogoItem) (ResultadoInterpretacao, error) {
	return ResultadoInterpretacao{}, errors.New("integração com Claude ainda não implementada — configure IA_PROVIDER=mock ou implemente esta função com uma chave de API real")
}
