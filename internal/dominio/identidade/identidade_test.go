package identidade

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestNovaV7(t *testing.T) {
	agora := time.UnixMilli(0x0192_6A5C_1234) // valor com todos os 6 bytes preenchidos
	zeros := bytes.NewReader(make([]byte, 10))

	id, err := NovaV7(agora, zeros)
	if err != nil {
		t.Fatalf("NovaV7 devolveu erro: %v", err)
	}

	if querido := "01926a5c-1234-7000-8000-000000000000"; id.String() != querido {
		t.Errorf("NovaV7 = %s, queria %s", id, querido)
	}
	if id[6]>>4 != 7 {
		t.Errorf("versao = %d, queria 7", id[6]>>4)
	}
	if id[8]>>6 != 0b10 {
		t.Errorf("variante = %b, queria 10", id[8]>>6)
	}
}

func TestNovaV7OrdenaPorTempo(t *testing.T) {
	antes, _ := NovaV7(time.UnixMilli(1000), rand.Reader)
	depois, _ := NovaV7(time.UnixMilli(1001), rand.Reader)

	// A propriedade que justifica v7 em vez de v4: ordem lexica = ordem temporal.
	if bytes.Compare(antes[:], depois[:]) >= 0 {
		t.Errorf("id de 1000ms (%s) deveria vir antes do de 1001ms (%s)", antes, depois)
	}
}

func TestNovaV7FonteQuebrada(t *testing.T) {
	curta := bytes.NewReader([]byte{1, 2, 3})
	if _, err := NovaV7(time.Now(), curta); err == nil {
		t.Error("fonte de aleatoriedade insuficiente deveria devolver erro")
	}
}

func TestAnalisar(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		erro    error
	}{
		{"valido", "01926a5c-1234-7000-8000-000000000001", nil},
		{"maiusculas", "01926A5C-1234-7000-8000-000000000001", nil},
		{"curto", "01926a5c-1234-7000-8000", ErrFormato},
		{"sem hifens", "01926a5c12347000800000000000000001", ErrFormato},
		{"hifen fora do lugar", "01926a5c1-234-7000-8000-000000000001", ErrFormato},
		{"nao hexadecimal", "01926a5c-1234-7000-8000-00000000000g", ErrFormato},
		{"vazio", "", ErrFormato},
		{"nulo", "00000000-0000-0000-0000-000000000000", ErrVazio},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			id, err := Analisar(c.entrada)
			if !errors.Is(err, c.erro) {
				t.Fatalf("Analisar(%q) erro = %v, queria %v", c.entrada, err, c.erro)
			}
			if err == nil && id.EhZero() {
				t.Errorf("Analisar(%q) aceitou mas devolveu id zero", c.entrada)
			}
		})
	}
}

func TestIdaEVolta(t *testing.T) {
	original, err := NovaV7(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	obtido, err := Analisar(original.String())
	if err != nil || obtido != original {
		t.Errorf("ida e volta de %s virou %s (erro %v)", original, obtido, err)
	}
}
