package dinheiro

import (
	"errors"
	"math"
	"testing"
)

func TestAnalisar(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		querido Centavos
	}{
		{"simbolo e milhar", "R$ 1.234,56", 123456},
		{"sem simbolo", "1.234,56", 123456},
		{"sem milhar", "1234,56", 123456},
		{"sem centavos", "R$ 10", 1000},
		{"centavos zerados", "R$ 0,00", 0},
		{"so centavos", "0,07", 7},
		{"negativo antes do simbolo", "-R$ 47,90", -4790},
		{"negativo depois do simbolo", "R$ -47,90", -4790},
		{"positivo explicito", "+R$ 47,90", 4790},
		{"espaco sobrando", "   R$   1.000,00   ", 100000},
		{"simbolo colado", "R$1.000,00", 100000},
		{"milhao", "R$ 1.234.567,89", 123456789},
		{"grupo inicial curto", "R$ 12.345,00", 1234500},
		{"limite de int64", "92233720368547758,07", math.MaxInt64},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			obtido, err := Analisar(c.entrada)
			if err != nil {
				t.Fatalf("Analisar(%q) devolveu erro inesperado: %v", c.entrada, err)
			}
			if obtido != c.querido {
				t.Errorf("Analisar(%q) = %d, queria %d", c.entrada, obtido, c.querido)
			}
		})
	}
}

func TestAnalisarErro(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		querido error
	}{
		{"vazio", "", ErrVazio},
		{"so espaco", "   ", ErrVazio},
		{"texto", "abc", ErrFormato},
		{"letra no meio", "1.2a4,56", ErrFormato},
		{"ponto decimal americano", "1234.56", ErrFormato},
		{"um digito decimal", "R$ 1,5", ErrFormato},
		{"tres digitos decimais", "R$ 1,500", ErrFormato},
		{"duas virgulas", "R$ 1,50,50", ErrFormato},
		{"grupo de milhar errado", "R$ 1.2345,00", ErrFormato},
		{"grupo inicial longo", "R$ 1234.567,00", ErrFormato},
		{"parte inteira ausente", "R$ ,50", ErrFormato},
		{"ponto sobrando", "R$ 1..234,56", ErrFormato},
		{"termina em ponto", "R$ 1.,56", ErrFormato},
		{"sinal duplicado", "-R$ -10,00", ErrFormato},
		{"so o simbolo", "R$", ErrFormato},
		{"so o sinal", "-", ErrFormato},
		{"estouro de int64", "92233720368547758,08", ErrEstouro},
		{"muito acima do int64", "999999999999999999999,00", ErrEstouro},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			// errors.Is e nao ==: sobrevive a um erro que venha embrulhado com %w.
			if _, err := Analisar(c.entrada); !errors.Is(err, c.querido) {
				t.Errorf("Analisar(%q) devolveu %v, queria %v", c.entrada, err, c.querido)
			}
		})
	}
}

func TestString(t *testing.T) {
	casos := []struct {
		nome    string
		entrada Centavos
		querido string
	}{
		{"zero", 0, "R$ 0,00"},
		{"um centavo", 1, "R$ 0,01"},
		{"dez centavos", 10, "R$ 0,10"},
		{"um real", 100, "R$ 1,00"},
		{"milhar", 123456, "R$ 1.234,56"},
		{"negativo", -4790, "-R$ 47,90"},
		{"centavo negativo", -1, "-R$ 0,01"},
		{"milhao", 123456789, "R$ 1.234.567,89"},
		{"grupo exato", 100000000, "R$ 1.000.000,00"},
		{"maximo", math.MaxInt64, "R$ 92.233.720.368.547.758,07"},
		{"minimo", math.MinInt64, "-R$ 92.233.720.368.547.758,08"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if obtido := c.entrada.String(); obtido != c.querido {
				t.Errorf("Centavos(%d).String() = %q, queria %q", c.entrada, obtido, c.querido)
			}
		})
	}
}

// TestIdaEVolta prova que String e Analisar sao inversos. E a propriedade que
// impede o valor de se corromper ao atravessar texto (JSON, Telegram, extrato).
func TestIdaEVolta(t *testing.T) {
	valores := []Centavos{0, 1, -1, 100, -4790, 123456, -123456789, math.MaxInt64}

	for _, v := range valores {
		texto := v.String()
		obtido, err := Analisar(texto)
		if err != nil {
			t.Fatalf("Analisar(%q) devolveu erro: %v", texto, err)
		}
		if obtido != v {
			t.Errorf("ida e volta de %d passou por %q e virou %d", v, texto, obtido)
		}
	}
}

func TestSomar(t *testing.T) {
	casos := []struct {
		nome    string
		base    Centavos
		outros  []Centavos
		querido Centavos
	}{
		{"sem parcelas", 100, nil, 100},
		{"uma parcela", 100, []Centavos{50}, 150},
		{"varias parcelas", 0, []Centavos{1000, -4790, 250}, -3540},
		{"soma exata de centavos", 0, []Centavos{1, 1, 1}, 3},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if obtido := c.base.Somar(c.outros...); obtido != c.querido {
				t.Errorf("%d.Somar(%v) = %d, queria %d", c.base, c.outros, obtido, c.querido)
			}
		})
	}
}

// FuzzAnalisar nao verifica valor: verifica que nenhuma entrada causa panic e
// que o pacote nunca devolve valor e erro ao mesmo tempo.
func FuzzAnalisar(f *testing.F) {
	f.Add("R$ 1.234,56")
	f.Add("-47,90")
	f.Add("R$")
	f.Add("1.2345,00")

	f.Fuzz(func(t *testing.T, entrada string) {
		valor, err := Analisar(entrada)
		if err != nil && valor != 0 {
			t.Errorf("Analisar(%q) devolveu erro %v junto com o valor %d", entrada, err, valor)
		}
		if err == nil {
			// Todo valor aceito precisa sobreviver a ida e volta.
			if revalor, reerr := Analisar(valor.String()); reerr != nil || revalor != valor {
				t.Errorf("Analisar(%q) = %d, mas %q voltou como %d (erro %v)", entrada, valor, valor.String(), revalor, reerr)
			}
		}
	})
}
