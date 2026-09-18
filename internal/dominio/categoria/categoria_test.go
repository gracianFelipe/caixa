package categoria

import (
	"errors"
	"testing"
)

func TestNova(t *testing.T) {
	casos := []struct {
		nome    string
		id      ID
		texto   string
		querida Categoria
		erro    error
	}{
		{"valida", 1, "mercado", Categoria{1, "mercado"}, nil},
		{"espaco aparado", 2, "  restaurante  ", Categoria{2, "restaurante"}, nil},
		{"id zero", 0, "mercado", Categoria{}, ErrIDInvalido},
		{"id negativo", -1, "mercado", Categoria{}, ErrIDInvalido},
		{"nome vazio", 1, "   ", Categoria{}, ErrNomeVazio},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			obtida, err := Nova(c.id, c.texto)
			if !errors.Is(err, c.erro) {
				t.Fatalf("Nova erro = %v, queria %v", err, c.erro)
			}
			if obtida != c.querida {
				t.Errorf("Nova = %+v, queria %+v", obtida, c.querida)
			}
		})
	}
}
