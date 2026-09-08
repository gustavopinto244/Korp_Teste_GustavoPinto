package estoqueclient

import (
	"math/rand"
	"time"
)

// backoffDelay devolve o atraso a aguardar antes da tentativa seguinte.
// tentativa é o número da tentativa que acabou de falhar (1 para a
// primeira). Backoff exponencial com jitter: ~200ms antes da 2ª tentativa,
// ~500ms±jitter antes da 3ª.
func backoffDelay(tentativa int) time.Duration {
	if tentativa <= 1 {
		return 200 * time.Millisecond
	}
	base := 500 * time.Millisecond
	jitter := time.Duration(rand.Int63n(int64(200 * time.Millisecond)))
	return base + jitter
}
