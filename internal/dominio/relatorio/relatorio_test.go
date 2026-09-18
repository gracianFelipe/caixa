package relatorio

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// -update regrava os golden files: `go test ./internal/dominio/relatorio -update`.
var atualizarGolden = flag.Bool("update", false, "regrava os golden files")

var (
	alvo, _  = competencia.Nova(2026, time.September)
	saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)
	contador byte
)

// gasto cria uma saida confirmada e categorizada num dia do mes dado.
func gasto(t *testing.T, ano int, mes time.Month, dia int, cat categoria.ID, contraparte string, valor int64) lancamento.Lancamento {
	t.Helper()
	contador++
	id := identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, contador}
	l, err := lancamento.Novo(id, lancamento.Dados{
		OcorridoEm:  time.Date(ano, mes, dia, 12, 0, 0, 0, saoPaulo),
		Valor:       dinheiro.Centavos(valor),
		Meio:        lancamento.MeioPix,
		Contraparte: contraparte,
	}, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}
	if cat > 0 {
		l, _ = l.ComCategoria(cat, lancamento.CategoriaPorRegra)
	}
	return l
}

func janela(ls ...lancamento.Lancamento) Janela {
	return Janela{Alvo: alvo, Lancamentos: ls, Limites: map[categoria.ID]dinheiro.Centavos{}}
}

func sinaisDoTipo(sinais []Sinal, tipo TipoDeSinal) []Sinal {
	var out []Sinal
	for _, s := range sinais {
		if s.Tipo == tipo {
			out = append(out, s)
		}
	}
	return out
}

func TestDetectarRepeticao(t *testing.T) {
	varios := func(n int, valor int64) []lancamento.Lancamento {
		var ls []lancamento.Lancamento
		for i := 0; i < n; i++ {
			ls = append(ls, gasto(t, 2026, 9, 1+i, 2, "PADARIA", valor))
		}
		return ls
	}

	casos := []struct {
		nome    string
		ls      []lancamento.Lancamento
		dispara bool
	}{
		{"8 de R$ 20 somam 160", varios(8, -2000), true},
		{"7 de R$ 25 nao chega a 8", varios(7, -2500), false},
		{"8 de R$ 15 somam so 120", varios(8, -1500), false},
		{"8 de R$ 31 passam do teto unitario", varios(8, -3100), false},
		{"mes anterior nao conta", append(varios(4, -2000), gasto(t, 2026, 8, 1, 2, "PADARIA", -2000), gasto(t, 2026, 8, 2, 2, "PADARIA", -2000), gasto(t, 2026, 8, 3, 2, "PADARIA", -2000), gasto(t, 2026, 8, 4, 2, "PADARIA", -2000)), false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := DetectarRepeticao(janela(c.ls...))
			if (len(s) > 0) != c.dispara {
				t.Errorf("sinais = %+v, dispara = %v", s, c.dispara)
			}
		})
	}
}

func TestDetectarAssinatura(t *testing.T) {
	mensal := func(valores ...int64) []lancamento.Lancamento {
		// termina no alvo (setembro), volta um mes por valor, dia 5 fixo
		var ls []lancamento.Lancamento
		mes := time.September
		ano := 2026
		for i := len(valores) - 1; i >= 0; i-- {
			ls = append(ls, gasto(t, ano, mes, 5, 8, "STREAMING X", valores[i]))
			mes--
			if mes == 0 {
				mes = time.December
				ano--
			}
		}
		return ls
	}

	casos := []struct {
		nome    string
		ls      []lancamento.Lancamento
		cadeia  int
		dispara bool
	}{
		{"tres meses iguais", mensal(-3990, -3990, -3990), 3, true},
		{"dois meses so", mensal(-3990, -3990), 0, false},
		{"variacao de 5% passa", mensal(-4000, -4000, -4200), 3, true},
		{"variacao de 6% quebra a cadeia", mensal(-4000, -4000, -4300), 0, false},
		{"seis meses", mensal(-1990, -1990, -1990, -1990, -1990, -1990), 6, true},
		{"parou antes do alvo", func() []lancamento.Lancamento {
			ls := mensal(-3990, -3990, -3990)
			return ls[1:] // remove o de setembro: agosto vira o ultimo
		}(), 0, false},
		{"intervalo de 40 dias quebra", []lancamento.Lancamento{
			gasto(t, 2026, 6, 20, 8, "STREAMING X", -3990),
			gasto(t, 2026, 7, 30, 8, "STREAMING X", -3990),
			gasto(t, 2026, 9, 5, 8, "STREAMING X", -3990),
		}, 0, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := DetectarAssinatura(janela(c.ls...))
			if (len(s) > 0) != c.dispara {
				t.Fatalf("sinais = %+v, dispara = %v", s, c.dispara)
			}
			if c.dispara && s[0].Severidade != c.cadeia {
				t.Errorf("cadeia = %d, queria %d", s[0].Severidade, c.cadeia)
			}
		})
	}
}

func TestDetectarEscalada(t *testing.T) {
	serie := func(valores ...int64) []lancamento.Lancamento {
		var ls []lancamento.Lancamento
		mes := time.September
		ano := 2026
		for i := len(valores) - 1; i >= 0; i-- {
			ls = append(ls, gasto(t, ano, mes, 10, 3, "POSTO", valores[i]))
			mes--
			if mes == 0 {
				mes = time.December
				ano--
			}
		}
		return ls
	}

	casos := []struct {
		nome    string
		ls      []lancamento.Lancamento
		dispara bool
	}{
		{"tres meses subindo 50%", serie(-20000, -25000, -30000), true},
		{"alta de 24% nao dispara", serie(-20000, -22000, -24800), false},
		{"alta de 25% exata dispara", serie(-20000, -22000, -25000), true},
		{"dois meses so", serie(-20000, -30000), false},
		{"queda no meio quebra", serie(-20000, -18000, -30000), false},
		{"alvo abaixo de R$ 100", serie(-3000, -4000, -6000), false},
		{"platô nao e escalada", serie(-20000, -20000, -30000), false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := DetectarEscalada(janela(c.ls...))
			if (len(s) > 0) != c.dispara {
				t.Errorf("sinais = %+v, dispara = %v", s, c.dispara)
			}
		})
	}
}

func TestDetectarEstouro(t *testing.T) {
	j := janela(
		gasto(t, 2026, 9, 1, 1, "MERCADO", -50000),
		gasto(t, 2026, 9, 2, 1, "MERCADO", -40000),
		gasto(t, 2026, 9, 3, 2, "IFOOD", -10000),
	)
	j.Limites = map[categoria.ID]dinheiro.Centavos{1: 80000, 2: 10000, 3: 5000}

	s := DetectarEstouro(j)
	if len(s) != 1 || s[0].Categoria != 1 {
		t.Fatalf("sinais = %+v, queria so a categoria 1 (90000 > 80000; 10000 = 10000 nao estoura)", s)
	}
	if s[0].Severidade != 112 { // 90000*100/80000
		t.Errorf("severidade = %d, queria 112", s[0].Severidade)
	}
}

func TestDetectarAtipico(t *testing.T) {
	historico := func(n int) []lancamento.Lancamento {
		var ls []lancamento.Lancamento
		for i := 0; i < n; i++ {
			// ~R$ 40 a R$ 60 nos meses anteriores
			ls = append(ls, gasto(t, 2026, time.Month(3+i%6), 1+i%27, 1, "MERCADO", -(4000+int64(i%5)*500)))
		}
		return ls
	}

	t.Run("dispara com historico suficiente", func(t *testing.T) {
		ls := append(historico(12), gasto(t, 2026, 9, 10, 1, "ELETRO LOJA", -250000))
		s := DetectarAtipico(janela(ls...))
		if len(s) != 1 || s[0].Contraparte != "ELETRO LOJA" {
			t.Fatalf("sinais = %+v", s)
		}
	})
	t.Run("nove amostras nao bastam", func(t *testing.T) {
		ls := append(historico(9), gasto(t, 2026, 9, 10, 1, "ELETRO LOJA", -250000))
		if s := DetectarAtipico(janela(ls...)); len(s) != 0 {
			t.Errorf("sinais = %+v", s)
		}
	})
	t.Run("abaixo de R$ 100 nao dispara mesmo sendo outlier", func(t *testing.T) {
		var ls []lancamento.Lancamento
		for i := 0; i < 12; i++ {
			ls = append(ls, gasto(t, 2026, time.Month(3+i%6), 1+i, 1, "CAFE", -500))
		}
		ls = append(ls, gasto(t, 2026, 9, 10, 1, "CAFE GRANDE", -9000))
		if s := DetectarAtipico(janela(ls...)); len(s) != 0 {
			t.Errorf("sinais = %+v", s)
		}
	})
	t.Run("o proprio outlier nao contamina a mediana", func(t *testing.T) {
		// Um gasto enorme no historico nao impede detectar outro no alvo:
		// com media+desvio, impediria.
		ls := append(historico(12), gasto(t, 2026, 7, 15, 1, "OUTRO ENORME", -300000))
		ls = append(ls, gasto(t, 2026, 9, 10, 1, "ELETRO LOJA", -250000))
		if s := DetectarAtipico(janela(ls...)); len(s) != 1 {
			t.Errorf("sinais = %+v, queria detectar apesar do outlier historico", s)
		}
	})
}

func TestMediana(t *testing.T) {
	casos := []struct {
		valores []int64
		querida int64
	}{
		{[]int64{5}, 5},
		{[]int64{1, 3}, 2},
		{[]int64{3, 1, 2}, 2},
		{[]int64{1, 2, 3, 100}, 2},
	}
	for _, c := range casos {
		vs := make([]dinheiro.Centavos, len(c.valores))
		for i, v := range c.valores {
			vs[i] = dinheiro.Centavos(v)
		}
		if m := mediana(vs); int64(m) != c.querida {
			t.Errorf("mediana(%v) = %d, queria %d", c.valores, m, c.querida)
		}
	}
}

// TestGerarGolden congela um cenario que dispara os cinco detectores. Diff
// no golden = mudanca de comportamento visivel no code review.
func TestGerarGolden(t *testing.T) {
	contador = 100
	var ls []lancamento.Lancamento

	// repeticao: 9 cafes de R$ 20 em restaurante (cat 2)
	for i := 0; i < 9; i++ {
		ls = append(ls, gasto(t, 2026, 9, 1+i, 2, "CAFE DA ESQUINA", -2000))
	}
	// assinatura: streaming (cat 8) 4 meses seguidos
	for _, m := range []time.Month{6, 7, 8, 9} {
		ls = append(ls, gasto(t, 2026, m, 5, 8, "STREAMING X", -3990))
	}
	// escalada: transporte (cat 3) 3 meses subindo
	ls = append(ls,
		gasto(t, 2026, 7, 10, 3, "POSTO", -20000),
		gasto(t, 2026, 8, 10, 3, "POSTO", -26000),
		gasto(t, 2026, 9, 10, 3, "POSTO", -32000),
	)
	// estouro: mercado (cat 1) limite 800, gasto 900
	ls = append(ls, gasto(t, 2026, 9, 12, 1, "SUPERMERCADO", -90000))
	// atipico: historico de 12 gastos pequenos + um enorme no alvo
	for i := 0; i < 12; i++ {
		ls = append(ls, gasto(t, 2026, time.Month(3+i%6), 2+i, 1, "MERCADINHO", -4500))
	}
	ls = append(ls, gasto(t, 2026, 9, 20, 7, "ELETRO LOJA", -250000))
	// entrada e mes anterior para os totais
	ls = append(ls,
		gasto(t, 2026, 9, 5, 12, "EMPRESA", 500000),
		gasto(t, 2026, 8, 20, 1, "SUPERMERCADO", -70000),
	)

	j := janela(ls...)
	j.Limites = map[categoria.ID]dinheiro.Centavos{1: 80000}

	r := Gerar(j)

	tipos := map[TipoDeSinal]bool{}
	for _, s := range r.Sinais {
		tipos[s.Tipo] = true
	}
	for _, tipo := range []TipoDeSinal{SinalRepeticao, SinalAssinatura, SinalEscalada, SinalEstouro, SinalAtipico} {
		if !tipos[tipo] {
			t.Errorf("cenario completo nao disparou %s", tipo)
		}
	}

	nomes := map[categoria.ID]string{1: "mercado", 2: "restaurante", 3: "transporte", 7: "lazer", 8: "assinatura", 12: "renda"}
	compararGolden(t, "relatorio_completo.golden.json", r)
	compararGoldenTexto(t, "relatorio_completo.golden.txt", Formatar(r, nomes))
}

func compararGolden(t *testing.T, nome string, v any) {
	t.Helper()
	caminho := filepath.Join("testdata", nome)
	obtido, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	obtido = append(obtido, '\n')

	if *atualizarGolden {
		if err := os.WriteFile(caminho, obtido, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	querido, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("golden ausente (rode com -update): %v", err)
	}
	if diff := cmp.Diff(string(querido), string(obtido)); diff != "" {
		t.Errorf("golden %s divergiu (-golden +obtido):\n%s", nome, diff)
	}
}

func compararGoldenTexto(t *testing.T, nome, obtido string) {
	t.Helper()
	caminho := filepath.Join("testdata", nome)
	if *atualizarGolden {
		if err := os.WriteFile(caminho, []byte(obtido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	querido, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("golden ausente (rode com -update): %v", err)
	}
	if diff := cmp.Diff(string(querido), obtido); diff != "" {
		t.Errorf("golden %s divergiu (-golden +obtido):\n%s", nome, diff)
	}
}
