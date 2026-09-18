package categorizacao

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
)

func regra(t *testing.T, id int64, cat categoria.ID, tipo Tipo, padrao string, prioridade int16) Regra {
	t.Helper()
	r, err := NovaRegra(id, cat, tipo, padrao, prioridade)
	if err != nil {
		t.Fatalf("NovaRegra(%d, %s, %q): %v", id, tipo, padrao, err)
	}
	return r
}

func TestNovaRegraErro(t *testing.T) {
	casos := []struct {
		nome   string
		cat    categoria.ID
		tipo   Tipo
		padrao string
		erro   error
	}{
		{"categoria zero", 0, TipoExata, "X", ErrCategoriaVazia},
		{"padrao vazio", 1, TipoContem, "  ", ErrPadraoVazio},
		{"tipo desconhecido", 1, "sufixo", "X", ErrTipoInvalido},
		{"regex quebrada", 1, TipoRegex, "IFOO[D", ErrRegexInvalida},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := NovaRegra(1, c.cat, c.tipo, c.padrao, 0); !errors.Is(err, c.erro) {
				t.Errorf("NovaRegra devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

func TestCorresponde(t *testing.T) {
	casos := []struct {
		nome        string
		tipo        Tipo
		padrao      string
		contraparte string
		casa        bool
	}{
		{"exata igual", TipoExata, "EXTRA", "EXTRA", true},
		{"exata nao e substring", TipoExata, "EXTRA", "EXTRATO BANCARIO", false},
		{"prefixo", TipoPrefixo, "AMAZON PRIME", "AMAZON PRIME VIDEO", true},
		{"prefixo no meio nao vale", TipoPrefixo, "PRIME", "AMAZON PRIME", false},
		{"contem", TipoContem, "IFOOD", "IFD IFOOD RESTAURANTE", true},
		{"contem em minusculas do padrao", TipoContem, "ifood", "PEDIDO IFOOD", true},
		{"contem ausente", TipoContem, "UBER", "IFOOD", false},
		{"regex", TipoRegex, "^UBER( TRIP)?$", "UBER TRIP", true},
		{"regex sem casar", TipoRegex, "^UBER$", "UBER TRIP", false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := regra(t, 1, 3, c.tipo, c.padrao, 0)
			if got := r.Corresponde(c.contraparte); got != c.casa {
				t.Errorf("Corresponde(%q) = %v, queria %v", c.contraparte, got, c.casa)
			}
		})
	}
}

func TestClassificarPrecedencia(t *testing.T) {
	casos := []struct {
		nome        string
		regras      []Regra
		contraparte string
		querida     categoria.ID
		regraID     int64
	}{
		{
			"prioridade vence tipo",
			[]Regra{
				regra(t, 1, 1, TipoExata, "IFOOD", 0),
				regra(t, 2, 2, TipoContem, "IFOOD", 10),
			},
			"IFOOD", 2, 2,
		},
		{
			"tipo mais especifico vence na mesma prioridade",
			[]Regra{
				regra(t, 1, 1, TipoContem, "IFOOD", 0),
				regra(t, 2, 2, TipoExata, "IFOOD", 0),
			},
			"IFOOD", 2, 2,
		},
		{
			"padrao mais longo vence no mesmo tipo",
			[]Regra{
				regra(t, 1, 1, TipoContem, "PRIME", 0),
				regra(t, 2, 8, TipoContem, "AMAZON PRIME", 0),
			},
			"AMAZON PRIME VIDEO", 8, 2,
		},
		{
			"menor id desempata tudo",
			[]Regra{
				regra(t, 9, 1, TipoContem, "MERCADO", 0),
				regra(t, 3, 2, TipoContem, "SUPERME", 0), // mesmo tamanho de padrao
			},
			"SUPERMERCADO XYZ", 2, 3,
		},
		{
			"regex so ganha de ninguem",
			[]Regra{
				regra(t, 1, 7, TipoRegex, "UBER", 0),
				regra(t, 2, 3, TipoContem, "UBER", 0),
			},
			"UBER TRIP", 3, 2,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			resultado, ok := Classificar(c.contraparte, c.regras)
			if !ok {
				t.Fatal("Classificar nao encontrou regra")
			}
			if resultado.Categoria != c.querida || resultado.RegraID != c.regraID {
				t.Errorf("Classificar = (%d, regra %d), queria (%d, regra %d)",
					resultado.Categoria, resultado.RegraID, c.querida, c.regraID)
			}
		})
	}
}

func TestClassificarSemRegra(t *testing.T) {
	regras := []Regra{regra(t, 1, 1, TipoContem, "IFOOD", 0)}
	if _, ok := Classificar("SUPERMERCADO", regras); ok {
		t.Error("nao deveria classificar sem correspondencia")
	}
	if _, ok := Classificar("QUALQUER", nil); ok {
		t.Error("lista vazia de regras nao classifica")
	}
}

// TestClassificarEhDeterministico embaralha as regras muitas vezes e exige o
// mesmo resultado: a promessa "nunca empate nao resolvido" virando asseracao.
func TestClassificarEhDeterministico(t *testing.T) {
	regras := []Regra{
		regra(t, 1, 1, TipoContem, "MERCADO", 0),
		regra(t, 2, 2, TipoContem, "SUPERMERCADO", 0),
		regra(t, 3, 3, TipoPrefixo, "SUPER", 0),
		regra(t, 4, 4, TipoExata, "SUPERMERCADO XYZ", 0),
		regra(t, 5, 5, TipoRegex, "MERC", 5),
	}

	primeira, ok := Classificar("SUPERMERCADO XYZ", regras)
	if !ok {
		t.Fatal("deveria classificar")
	}

	sorteio := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		sorteio.Shuffle(len(regras), func(a, b int) { regras[a], regras[b] = regras[b], regras[a] })
		resultado, ok := Classificar("SUPERMERCADO XYZ", regras)
		if !ok || resultado != primeira {
			t.Fatalf("iteracao %d: resultado %+v difere de %+v", i, resultado, primeira)
		}
	}
}
