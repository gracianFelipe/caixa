//go:build integracao

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

// semeiaEventos grava lancamentos (cada Salvar carrega um evento de outbox) e
// devolve os ids dos eventos pendentes criados.
func semeiaEventos(t *testing.T, repo *Repositorio, n int) map[identidade.ID]bool {
	t.Helper()
	ctx := context.Background()
	criados := make(map[identidade.ID]bool, n)

	for i := 0; i < n; i++ {
		l := novoLancamento(t, repo, time.Date(1997, time.February, 1+i, 12, 0, 0, 0, time.UTC))
		e := eventoPara(t, repo, l)
		if err := repo.Salvar(ctx, l, e); err != nil {
			t.Fatal(err)
		}
		criados[e.ID] = true
	}
	return criados
}

// TestConsumirPendentesConcorrente e a prova do SKIP LOCKED: dois consumidores
// segurando transacoes ao mesmo tempo recebem eventos disjuntos.
func TestConsumirPendentesConcorrente(t *testing.T) {
	repo := repositorioDeTeste(t)
	eventos := NovoRepositorioDeEventos(repo.pool)
	criados := semeiaEventos(t, repo, 4)

	var (
		mu        sync.Mutex
		vistos    = map[identidade.ID]int{}
		barreira  = make(chan struct{})
		prontos   sync.WaitGroup
		terminado sync.WaitGroup
	)

	consumidor := func() {
		defer terminado.Done()
		var umaVez sync.Once
		_, err := eventos.ConsumirPendentes(context.Background(), 2, func(e evento.Evento) error {
			if criados[e.ID] {
				mu.Lock()
				vistos[e.ID]++
				mu.Unlock()
			}
			// Sinaliza (uma vez) que trancou o lote e espera o outro
			// consumidor trancar o dele — as duas transacoes ficam abertas
			// AO MESMO TEMPO, que e o cenario que o SKIP LOCKED resolve.
			umaVez.Do(prontos.Done)
			<-barreira
			return nil
		})
		if err != nil {
			t.Errorf("ConsumirPendentes: %v", err)
		}
	}

	prontos.Add(2)
	terminado.Add(2)
	go consumidor()
	go consumidor()

	prontos.Wait()  // cada um trancou pelo menos o primeiro evento do lote
	close(barreira) // libera os dois para terminar
	terminado.Wait()

	// O banco de dev pode ter eventos alheios pendentes; drena o resto ate
	// os nossos quatro aparecerem, sem barreira.
	for i := 0; i < 5 && len(vistos) < 4; i++ {
		if _, err := eventos.ConsumirPendentes(context.Background(), 100, func(e evento.Evento) error {
			if criados[e.ID] {
				mu.Lock()
				vistos[e.ID]++
				mu.Unlock()
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	for id, n := range vistos {
		if n != 1 {
			t.Errorf("evento %s processado %d vezes: SKIP LOCKED falhou", id, n)
		}
	}
	if len(vistos) != 4 {
		t.Errorf("processados %d dos 4 eventos", len(vistos))
	}

	// Ultima passada: nenhum dos nossos pode continuar pendente.
	if _, err := eventos.ConsumirPendentes(context.Background(), 100, func(e evento.Evento) error {
		if criados[e.ID] {
			t.Errorf("evento %s ainda pendente apos consumo", e.ID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestConsumirPendentesErroDevolveEventos: falha no processamento faz rollback
// e os eventos continuam pendentes para a proxima rodada.
func TestConsumirPendentesErroDevolveEventos(t *testing.T) {
	repo := repositorioDeTeste(t)
	eventos := NovoRepositorioDeEventos(repo.pool)
	criados := semeiaEventos(t, repo, 1)

	falha := context.DeadlineExceeded // qualquer erro serve
	_, err := eventos.ConsumirPendentes(context.Background(), 100, func(e evento.Evento) error {
		if criados[e.ID] {
			return falha
		}
		return nil
	})
	if err == nil {
		t.Fatal("esperava o erro do processador")
	}

	// O evento tem que continuar pendente.
	achado := false
	_, err = eventos.ConsumirPendentes(context.Background(), 100, func(e evento.Evento) error {
		if criados[e.ID] {
			achado = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !achado {
		t.Error("evento sumiu da fila apesar do rollback")
	}
}
