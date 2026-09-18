package pergunta

import (
	"errors"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

func TestNova(t *testing.T) {
	agora := time.Now()
	idP, idL := identidade.ID{1}, identidade.ID{2}

	p, err := Nova(idP, idL, 777, agora)
	if err != nil {
		t.Fatalf("Nova devolveu erro: %v", err)
	}
	if p.Estado != Aberta || p.MensagemID != 0 {
		t.Errorf("pergunta nova = %+v, queria aberta e sem mensagem", p)
	}

	casos := []struct {
		nome string
		id   identidade.ID
		l    identidade.ID
		chat int64
		erro error
	}{
		{"id vazio", identidade.ID{}, idL, 777, ErrIDVazio},
		{"lancamento vazio", idP, identidade.ID{}, 777, ErrSemLancamento},
		{"chat zero", idP, idL, 0, ErrChatInvalido},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := Nova(c.id, c.l, c.chat, agora); !errors.Is(err, c.erro) {
				t.Errorf("Nova devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

func TestNovaDeConciliacao(t *testing.T) {
	agora := time.Now()
	idP, idL, idRef := identidade.ID{1}, identidade.ID{2}, identidade.ID{3}

	p, err := NovaDeConciliacao(idP, idL, idRef, 777, agora)
	if err != nil {
		t.Fatalf("NovaDeConciliacao: %v", err)
	}
	if p.Tipo != TipoConciliacao || p.Referencia != idRef {
		t.Errorf("pergunta = %+v", p)
	}

	if _, err := NovaDeConciliacao(idP, idL, identidade.ID{}, 777, agora); !errors.Is(err, ErrSemReferencia) {
		t.Errorf("sem referencia: %v", err)
	}
}
