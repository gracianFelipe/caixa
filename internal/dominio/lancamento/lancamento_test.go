package lancamento

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

var saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)

// idFixo evita aleatoriedade no teste: o id e entrada, nao comportamento.
var idFixo = identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 1}

func dadosValidos() Dados {
	return Dados{
		OcorridoEm:  time.Date(2026, time.September, 17, 15, 0, 0, 0, time.UTC),
		Valor:       -4790,
		Meio:        MeioPix,
		Contraparte: "  Supermercado XYZ 0042 ",
	}
}

func TestNovo(t *testing.T) {
	setembro, _ := competencia.Nova(2026, time.September)

	obtido, err := Novo(idFixo, dadosValidos(), saoPaulo)
	if err != nil {
		t.Fatalf("Novo devolveu erro inesperado: %v", err)
	}

	querido := Lancamento{
		ID:              idFixo,
		OcorridoEm:      time.Date(2026, time.September, 17, 15, 0, 0, 0, time.UTC),
		Competencia:     setembro,
		Valor:           -4790,
		Meio:            MeioPix,
		Contraparte:     "Supermercado XYZ 0042",
		ContraparteNorm: "SUPERMERCADO XYZ",
		CategoriaOrigem: CategoriaPendente,
	}

	// cmp.Diff mostra so o campo que divergiu; com != voce ve dois structs inteiros.
	// AllowUnexported: Competencia guarda os campos privados de proposito.
	if diff := cmp.Diff(querido, obtido, cmp.AllowUnexported(competencia.Competencia{})); diff != "" {
		t.Errorf("Novo divergiu (-querido +obtido):\n%s", diff)
	}
}

func TestNovoConverteParaUTC(t *testing.T) {
	d := dadosValidos()
	d.OcorridoEm = time.Date(2026, time.September, 30, 22, 30, 0, 0, saoPaulo)

	obtido, err := Novo(idFixo, d, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}

	if obtido.OcorridoEm.Location() != time.UTC {
		t.Errorf("OcorridoEm ficou em %v, queria UTC", obtido.OcorridoEm.Location())
	}
	// 22:30 de 30/09 em Sao Paulo e 01:30 UTC de 01/10, mas a competencia e setembro.
	if c := obtido.Competencia.String(); c != "2026-09" {
		t.Errorf("Competencia = %s, queria 2026-09", c)
	}
	if obtido.OcorridoEm.Day() != 1 || obtido.OcorridoEm.Month() != time.October {
		t.Errorf("OcorridoEm em UTC = %v, queria 01/10", obtido.OcorridoEm)
	}
}

func TestNovoErro(t *testing.T) {
	casos := []struct {
		nome   string
		id     identidade.ID
		ajuste func(*Dados)
		erro   error
	}{
		{"id vazio", identidade.ID{}, func(*Dados) {}, ErrIDVazio},
		{"instante zero", idFixo, func(d *Dados) { d.OcorridoEm = time.Time{} }, ErrInstanteZero},
		{"valor zero", idFixo, func(d *Dados) { d.Valor = 0 }, ErrValorZero},
		{"meio vazio", idFixo, func(d *Dados) { d.Meio = "" }, ErrMeioInvalido},
		{"meio desconhecido", idFixo, func(d *Dados) { d.Meio = "cheque" }, ErrMeioInvalido},
		{"contraparte vazia", idFixo, func(d *Dados) { d.Contraparte = "" }, ErrContraparteVazia},
		{"contraparte so espaco", idFixo, func(d *Dados) { d.Contraparte = "   " }, ErrContraparteVazia},
		{"contraparte longa", idFixo, func(d *Dados) { d.Contraparte = strings.Repeat("a", ContraparteMaxima+1) }, ErrContraparteLonga},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d := dadosValidos()
			c.ajuste(&d)
			if _, err := Novo(c.id, d, saoPaulo); !errors.Is(err, c.erro) {
				t.Errorf("Novo devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

func TestComCategoria(t *testing.T) {
	base, err := Novo(idFixo, dadosValidos(), saoPaulo)
	if err != nil {
		t.Fatal(err)
	}

	categorizado, err := base.ComCategoria(2, CategoriaPorRegra)
	if err != nil {
		t.Fatalf("ComCategoria devolveu erro: %v", err)
	}
	if categorizado.CategoriaID != 2 || categorizado.CategoriaOrigem != CategoriaPorRegra {
		t.Errorf("categoria = (%d, %s)", categorizado.CategoriaID, categorizado.CategoriaOrigem)
	}
	// Imutabilidade: o original nao muda.
	if base.CategoriaID != 0 || base.CategoriaOrigem != CategoriaPendente {
		t.Error("ComCategoria mutou o lancamento original")
	}

	casosDeErro := []struct {
		nome   string
		id     int16
		origem OrigemDaCategoria
		erro   error
	}{
		{"categoria zero", 0, CategoriaManual, ErrCategoriaInvalida},
		{"categoria negativa", -1, CategoriaManual, ErrCategoriaInvalida},
		{"origem pendente com categoria", 2, CategoriaPendente, ErrOrigemDeCategoria},
		{"origem inventada", 2, "chute", ErrOrigemDeCategoria},
	}
	for _, c := range casosDeErro {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := base.ComCategoria(categoria.ID(c.id), c.origem); !errors.Is(err, c.erro) {
				t.Errorf("ComCategoria devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

func TestNovoNormalizaMeio(t *testing.T) {
	d := dadosValidos()
	d.Meio = " PIX "

	obtido, err := Novo(idFixo, d, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}
	// O banco tem CHECK (meio IN ('pix', ...)); gravar " PIX " falharia la.
	if obtido.Meio != MeioPix {
		t.Errorf("Meio = %q, queria %q", obtido.Meio, MeioPix)
	}
}

func TestNovoContraparteNoLimite(t *testing.T) {
	d := dadosValidos()
	d.Contraparte = strings.Repeat("é", ContraparteMaxima) // limite conta runas, nao bytes

	if _, err := Novo(idFixo, d, saoPaulo); err != nil {
		t.Errorf("contraparte com exatamente %d runas deveria passar, deu %v", ContraparteMaxima, err)
	}
}

func TestAnalisarMeio(t *testing.T) {
	casos := []struct {
		entrada string
		querido Meio
		erro    error
	}{
		{"pix", MeioPix, nil},
		{"PIX", MeioPix, nil},
		{" credito ", MeioCredito, nil},
		{"transferencia", MeioTransferencia, nil},
		{"crédito", "", ErrMeioInvalido},
		{"cheque", "", ErrMeioInvalido},
		{"", "", ErrMeioInvalido},
	}

	for _, c := range casos {
		t.Run(c.entrada, func(t *testing.T) {
			obtido, err := AnalisarMeio(c.entrada)
			if !errors.Is(err, c.erro) || obtido != c.querido {
				t.Errorf("AnalisarMeio(%q) = (%q, %v), queria (%q, %v)", c.entrada, obtido, err, c.querido, c.erro)
			}
		})
	}
}

func TestEhSaida(t *testing.T) {
	saida, _ := Novo(idFixo, dadosValidos(), saoPaulo)
	if !saida.EhSaida() {
		t.Error("valor negativo deveria ser saida")
	}

	d := dadosValidos()
	d.Valor = 350000
	entrada, _ := Novo(idFixo, d, saoPaulo)
	if entrada.EhSaida() {
		t.Error("valor positivo nao deveria ser saida")
	}
}

func TestNormalizar(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		querido string
	}{
		{"acento e cedilha", "Pão de Açúcar", "PAO DE ACUCAR"},
		{"digitos removidos", "UBER *TRIP 8821", "UBER TRIP"},
		{"espacos colapsados", "  IFOOD   *  RESTAURANTE  ", "IFOOD RESTAURANTE"},
		{"pontuacao vira espaco", "NETFLIX.COM", "NETFLIX COM"},
		{"so digitos", "123456", ""},
		{"vazio", "", ""},
		{"minusculas", "farmacia sao joao", "FARMACIA SAO JOAO"},
		{"digito colado em letra", "POSTO7ESTRELAS", "POSTOESTRELAS"},
		{"n com til", "PEÑA", "PENA"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if obtido := Normalizar(c.entrada); obtido != c.querido {
				t.Errorf("Normalizar(%q) = %q, queria %q", c.entrada, obtido, c.querido)
			}
		})
	}
}
