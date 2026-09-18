//go:build integracao

// Roda so com `go test -tags=integracao ./...` e CAIXA_BD_URL definida.
// Sem a tag, o pacote compila e o `go test ./...` comum fica rapido e sem banco.

package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// eventoPara cria o evento de outbox que Salvar exige e registra a limpeza.
func eventoPara(t *testing.T, repo *Repositorio, l lancamento.Lancamento) evento.Evento {
	t.Helper()
	id, err := identidade.NovaV7(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	e, err := evento.Novo(id, evento.LancamentoCriado, l.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(context.Background(), "DELETE FROM eventos WHERE id = $1", id)
	})
	return e
}

var saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)

func repositorioDeTeste(t *testing.T) *Repositorio {
	t.Helper()
	url := os.Getenv("CAIXA_BD_URL")
	if url == "" {
		t.Skip("CAIXA_BD_URL nao definida; carregue local.ps1")
	}

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	pool, err := Conectar(ctx, url)
	if err != nil {
		t.Fatalf("conectando: %v", err)
	}
	t.Cleanup(pool.Close)
	return NovoRepositorio(pool)
}

// novoLancamento cria um agregado valido com id fresco. O chamador registra a
// limpeza para o banco nao acumular lixo entre execucoes.
func novoLancamento(t *testing.T, repo *Repositorio, ocorridoEm time.Time) lancamento.Lancamento {
	t.Helper()
	id, err := identidade.NovaV7(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	l, err := lancamento.Novo(id, lancamento.Dados{
		OcorridoEm:  ocorridoEm,
		Valor:       -4790,
		Meio:        lancamento.MeioPix,
		Contraparte: "Teste de Integração 42",
	}, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(context.Background(), "DELETE FROM lancamentos WHERE id = $1", id)
	})
	return l
}

func TestSalvarEDaCompetencia(t *testing.T) {
	repo := repositorioDeTeste(t)
	ctx := context.Background()

	// Usa um mes remoto para nao colidir com dados reais do usuario.
	segundo := novoLancamento(t, repo, time.Date(1999, time.March, 20, 12, 0, 0, 0, time.UTC))
	primeiro := novoLancamento(t, repo, time.Date(1999, time.March, 10, 12, 0, 0, 0, time.UTC))
	outroMes := novoLancamento(t, repo, time.Date(1999, time.April, 1, 12, 0, 0, 0, time.UTC))

	for _, l := range []lancamento.Lancamento{segundo, primeiro, outroMes} {
		if err := repo.Salvar(ctx, l, eventoPara(t, repo, l)); err != nil {
			t.Fatalf("Salvar: %v", err)
		}
	}

	marco, _ := competencia.Nova(1999, time.March)
	obtidos, err := repo.DaCompetencia(ctx, marco)
	if err != nil {
		t.Fatalf("DaCompetencia: %v", err)
	}

	// Ordem cronologica, so o mes pedido, e todos os campos intactos na volta.
	queridos := []lancamento.Lancamento{primeiro, segundo}
	if diff := cmp.Diff(queridos, obtidos, cmp.AllowUnexported(competencia.Competencia{})); diff != "" {
		t.Errorf("ida e volta pelo banco divergiu (-querido +obtido):\n%s", diff)
	}
}

func TestSalvarDuplicado(t *testing.T) {
	repo := repositorioDeTeste(t)
	ctx := context.Background()

	l := novoLancamento(t, repo, time.Date(1999, time.May, 1, 12, 0, 0, 0, time.UTC))
	if err := repo.Salvar(ctx, l, eventoPara(t, repo, l)); err != nil {
		t.Fatal(err)
	}

	// A chave primaria e a primeira linha de defesa contra duplicata.
	err := repo.Salvar(ctx, l, eventoPara(t, repo, l))
	if err == nil {
		t.Fatal("segundo Salvar do mesmo id deveria falhar")
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("erro deveria ser de constraint, nao de contexto: %v", err)
	}
}

func TestDaCompetenciaVazia(t *testing.T) {
	repo := repositorioDeTeste(t)

	remoto, _ := competencia.Nova(1901, time.January)
	obtidos, err := repo.DaCompetencia(context.Background(), remoto)
	if err != nil {
		t.Fatal(err)
	}
	if len(obtidos) != 0 {
		t.Errorf("competencia sem dados devolveu %d lancamentos", len(obtidos))
	}
}
