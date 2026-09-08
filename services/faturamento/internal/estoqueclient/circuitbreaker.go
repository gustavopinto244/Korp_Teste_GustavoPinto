package estoqueclient

import (
	"sync"
	"time"
)

// estadoCircuito representa os três estados clássicos de um circuit breaker.
type estadoCircuito int

const (
	fechado estadoCircuito = iota
	aberto
	meioAberto
)

// circuitBreaker é uma implementação própria, simples, protegida por mutex:
// abre após um número de falhas consecutivas de rede/timeout, permanece
// aberto por um intervalo fixo recusando chamadas imediatamente (sem gastar
// o timeout de tentativa), depois vai a half-open e libera uma única chamada
// de teste.
type circuitBreaker struct {
	mu sync.Mutex

	limiteFalhas     int
	duracaoAberto    time.Duration
	falhasSeguidas   int
	estado           estadoCircuito
	abriuEm          time.Time
	testeEmAndamento bool
}

func novoCircuitBreaker(limiteFalhas int, duracaoAberto time.Duration) *circuitBreaker {
	return &circuitBreaker{
		limiteFalhas:  limiteFalhas,
		duracaoAberto: duracaoAberto,
		estado:        fechado,
	}
}

// permiteChamada decide se uma nova chamada pode prosseguir. Quando o
// circuito está aberto mas o intervalo já passou, transiciona para
// half-open e libera exatamente uma chamada de teste por vez.
func (cb *circuitBreaker) permiteChamada() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.estado {
	case fechado:
		return true
	case meioAberto:
		if cb.testeEmAndamento {
			return false
		}
		cb.testeEmAndamento = true
		return true
	case aberto:
		if time.Since(cb.abriuEm) >= cb.duracaoAberto {
			cb.estado = meioAberto
			cb.testeEmAndamento = true
			return true
		}
		return false
	default:
		return true
	}
}

// registrarSucesso fecha o circuito e zera o contador de falhas.
func (cb *circuitBreaker) registrarSucesso() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.falhasSeguidas = 0
	cb.estado = fechado
	cb.testeEmAndamento = false
}

// registrarFalha incrementa o contador de falhas de rede/timeout e abre o
// circuito quando o limite é atingido. Uma falha durante o teste half-open
// reabre o circuito imediatamente.
func (cb *circuitBreaker) registrarFalha() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.estado == meioAberto {
		cb.abrir()
		return
	}

	cb.falhasSeguidas++
	if cb.falhasSeguidas >= cb.limiteFalhas {
		cb.abrir()
	}
}

// abrir deve ser chamado com cb.mu já travado.
func (cb *circuitBreaker) abrir() {
	cb.estado = aberto
	cb.abriuEm = time.Now()
	cb.testeEmAndamento = false
}
