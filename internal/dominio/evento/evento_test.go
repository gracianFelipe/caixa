package evento

import (
	"errors"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

var (
	idA = identidade.ID{1}
	idB = identidade.ID{2}
)

func TestNovo(t *testing.T) {
	agora := time.Now()

	e, err := Novo(idA, LancamentoCriado, idB, agora)
	if err != nil {
		t.Fatalf("Novo devolveu erro: %v", err)
	}
	if e.Tipo != LancamentoCriado || e.LancamentoID != idB || !e.CriadoEm.Equal(agora) {
		t.Errorf("Novo = %+v", e)
	}

	casos := []struct {
		nome string
		id   identidade.ID
		tipo Tipo
		alvo identidade.ID
		erro error
	}{
		{"id vazio", identidade.ID{}, LancamentoCriado, idB, ErrIDVazio},
		{"tipo desconhecido", idA, "cafe_pronto", idB, ErrTipoInvalido},
		{"sem assunto", idA, LancamentoCriado, identidade.ID{}, ErrSemAssunto},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := Novo(c.id, c.tipo, c.alvo, agora); !errors.Is(err, c.erro) {
				t.Errorf("Novo devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}
