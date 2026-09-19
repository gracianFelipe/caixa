package extrato

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

func fusoDeTeste(t *testing.T) *time.Location {
	t.Helper()
	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("carregando fuso: %v", err)
	}
	return fuso
}

func TestAnalisarCSVFixtureSintetica(t *testing.T) {
	dados, err := os.ReadFile("testdata/sintetico_bradesco.csv")
	if err != nil {
		t.Fatalf("lendo fixture: %v", err)
	}
	fuso := fusoDeTeste(t)

	transacoes, err := AnalisarCSV(dados, fuso)
	if err != nil {
		t.Fatalf("AnalisarCSV: %v", err)
	}

	// A linha COD. LANC. 0 esta zerada e nao vira lancamento; cabecalho do
	// banco, titulos e rodape tambem saem. Da recapitulacao entram SO as duas
	// linhas ineditas (dias 07 e 08); as repetidas (05 e 06) sao abatidas.
	if len(transacoes) != 9 {
		t.Fatalf("transacoes = %d, quero 9", len(transacoes))
	}

	quero := []struct {
		dia         int
		valor       dinheiro.Centavos
		meio        lancamento.Meio
		contraparte string
	}{
		{1, 123456, lancamento.MeioPix, "PIX RECEBIDO"},
		{2, -5000, lancamento.MeioPix, "PIX ENVIADO"},
		{2, -5000, lancamento.MeioPix, "PIX ENVIADO"},
		{3, -10000, lancamento.MeioBoleto, "PAGTO ELETRON COBRANCA"},
		{4, -2000, lancamento.MeioPix, "SAQUE CARTAO TRANSF PIX*"},
		{5, -1456, lancamento.MeioCredito, "GASTOS CARTAO DE CREDITO"},
		{6, -100000, lancamento.MeioTransferencia, "TRANSF.MMA.TITULARIDADE*"},
		{7, 3000, lancamento.MeioPix, "PIX RECEBIDO"},
		{8, -1000, lancamento.MeioPix, "TRANSFERENCIA PIX"},
	}

	for i, q := range quero {
		got := transacoes[i]
		if got.Valor != q.valor {
			t.Errorf("[%d] valor = %d, quero %d", i, got.Valor, q.valor)
		}
		if got.Meio != q.meio {
			t.Errorf("[%d] meio = %q, quero %q", i, got.Meio, q.meio)
		}
		if got.Contraparte != q.contraparte {
			t.Errorf("[%d] contraparte = %q, quero %q", i, got.Contraparte, q.contraparte)
		}
		if got.OcorridoEm.Day() != q.dia || got.OcorridoEm.Month() != time.March {
			t.Errorf("[%d] data = %s, quero dia %d de marco", i, got.OcorridoEm, q.dia)
		}
		// Sem hora no arquivo: meia-noite local, nunca UTC.
		if h, m, s := got.OcorridoEm.Clock(); h != 0 || m != 0 || s != 0 {
			t.Errorf("[%d] hora = %02d:%02d:%02d, quero 00:00:00", i, h, m, s)
		}
		if got.IDExterno != "" {
			t.Errorf("[%d] IDExterno = %q, quero vazio: CSV nao tem FITID", i, got.IDExterno)
		}
	}
}

// As duas linhas identicas do dia 02 tem de gerar payloads distintos, senao a
// constraint UNIQUE (origem_id, impressao) engoliria a segunda e um
// lancamento real sumiria.
func TestAnalisarCSVLinhasIdenticasRecebemOrdinais(t *testing.T) {
	dados, err := os.ReadFile("testdata/sintetico_bradesco.csv")
	if err != nil {
		t.Fatalf("lendo fixture: %v", err)
	}

	transacoes, err := AnalisarCSV(dados, fusoDeTeste(t))
	if err != nil {
		t.Fatalf("AnalisarCSV: %v", err)
	}

	primeira, segunda := transacoes[1], transacoes[2]
	if primeira.Payload == segunda.Payload {
		t.Fatalf("payloads iguais para linhas identicas: %q", primeira.Payload)
	}
	if !strings.HasSuffix(primeira.Payload, "#0") || !strings.HasSuffix(segunda.Payload, "#1") {
		t.Errorf("ordinais errados: %q e %q", primeira.Payload, segunda.Payload)
	}
}

// O payload alimenta a impressao gravada no banco: agencia e conta (linha 1
// do arquivo) nao podem aparecer nele.
func TestAnalisarCSVNaoVazaAgenciaEConta(t *testing.T) {
	dados, err := os.ReadFile("testdata/sintetico_bradesco.csv")
	if err != nil {
		t.Fatalf("lendo fixture: %v", err)
	}

	transacoes, err := AnalisarCSV(dados, fusoDeTeste(t))
	if err != nil {
		t.Fatalf("AnalisarCSV: %v", err)
	}
	for i, tr := range transacoes {
		if strings.Contains(strings.ToLower(tr.Payload), "conta") ||
			strings.Contains(strings.ToLower(tr.Payload), "ag:") {
			t.Errorf("[%d] payload carrega dado de conta: %q", i, tr.Payload)
		}
	}
}

func TestAnalisarCSVEntradasInvalidas(t *testing.T) {
	const cabecalho = "Data;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$)\n"

	casos := []struct {
		nome    string
		entrada string
		quero   error
	}{
		{
			nome:    "sem cabecalho",
			entrada: "01/03/2026;PIX RECEBIDO;1;10,00; ;10,00\n",
			quero:   ErrCabecalhoCSV,
		},
		{
			nome:    "so cabecalho",
			entrada: cabecalho,
			quero:   ErrSemTransacoes,
		},
		{
			nome:    "linha com menos colunas",
			entrada: cabecalho + "01/03/2026;PIX RECEBIDO;1\n",
			quero:   ErrLinhaCurta,
		},
		{
			nome:    "data fora do formato",
			entrada: cabecalho + "2026-03-01;PIX RECEBIDO;1;10,00; ;10,00\n",
			quero:   ErrDataInvalida,
		},
		{
			nome:    "valor com tres casas decimais",
			entrada: cabecalho + "01/03/2026;PIX RECEBIDO;1;10,005; ;10,00\n",
			quero:   ErrValorInvalido,
		},
		{
			nome:    "credito e debito na mesma linha",
			entrada: cabecalho + "01/03/2026;PIX RECEBIDO;1;10,00;5,00;10,00\n",
			quero:   ErrDoisValores,
		},
		{
			nome:    "historico vazio",
			entrada: cabecalho + "01/03/2026; ;1;10,00; ;10,00\n",
			quero:   ErrCampoObrigatorio,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := AnalisarCSV([]byte(c.entrada), fusoDeTeste(t))
			if !errors.Is(err, c.quero) {
				t.Fatalf("erro = %v, quero %v", err, c.quero)
			}
			// O erro nao pode carregar o conteudo da linha: e PII.
			if err != nil && strings.Contains(err.Error(), "PIX RECEBIDO") {
				t.Errorf("erro vaza conteudo da linha: %v", err)
			}
		})
	}
}

func TestAnalisarCSVArquivoGrande(t *testing.T) {
	gigante := make([]byte, TamanhoMaximo+1)
	if _, err := AnalisarCSV(gigante, fusoDeTeste(t)); !errors.Is(err, ErrArquivoGrande) {
		t.Fatalf("erro = %v, quero %v", err, ErrArquivoGrande)
	}
}

func TestMeioDoHistorico(t *testing.T) {
	casos := []struct {
		historico string
		quero     lancamento.Meio
	}{
		{"PIX ENVIADO", lancamento.MeioPix},
		{"DEVOLUCAO PIX", lancamento.MeioPix},
		{"PIX QR CODE ESTATICO", lancamento.MeioPix},
		{"SAQUE CARTAO TRANSF PIX*", lancamento.MeioPix}, // PIX tem precedencia
		{"SAQUE ELETRONICO", lancamento.MeioDinheiro},
		{"TRANSF SALDO C/SAL P/CC", lancamento.MeioTransferencia},
		{"TED RECEBIDA", lancamento.MeioTransferencia},
		{"PAGTO ELETRON COBRANCA", lancamento.MeioBoleto},
		{"GASTOS CARTAO DE CREDITO", lancamento.MeioCredito},
		{"PARCELA CREDITO PESSOAL", lancamento.MeioCredito},
		{"EMPRESTIMO PESSOAL", lancamento.MeioCredito},
		{"RENTAB.INVEST FACILCRED*", lancamento.MeioDebito},
		// Palavra inteira, nunca substring: a auditoria pegou "TED" dentro de
		// LIMITED e "DOC" dentro de DOCERIA.
		{"COMPRA LIMITED SHOP", lancamento.MeioDebito},
		{"DOCERIA DA ESQUINA", lancamento.MeioDebito},
	}

	for _, c := range casos {
		t.Run(c.historico, func(t *testing.T) {
			if got := meioDoHistorico(c.historico); got != c.quero {
				t.Errorf("meio = %q, quero %q", got, c.quero)
			}
		})
	}
}

func FuzzAnalisarCSV(f *testing.F) {
	f.Add("Data;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$)\n01/03/2026;PIX;1;10,00; ;10,00\n")
	f.Add(";;Total;1,00;1,00;0,00;\n")
	f.Add("\xEF\xBB\xBFData;Histórico;Docto.;Crédito;Débito;Saldo\n")

	fuso := time.FixedZone("BRT", -3*60*60)
	f.Fuzz(func(t *testing.T, entrada string) {
		// Contrato do fuzz: nunca entra em panico e nunca devolve transacao
		// junto com erro. O conteudo nao importa, a robustez sim.
		transacoes, err := AnalisarCSV([]byte(entrada), fuso)
		if err != nil && transacoes != nil {
			t.Fatalf("erro %v com %d transacoes", err, len(transacoes))
		}
		for _, tr := range transacoes {
			if tr.Contraparte == "" {
				t.Fatal("transacao aceita com contraparte vazia")
			}
			if len([]rune(tr.Contraparte)) > lancamento.ContraparteMaxima {
				t.Fatal("contraparte acima do limite do dominio")
			}
		}
	})
}

// A secao "Ultimos Lancamentos" ora repete o fim da tabela principal, ora
// traz movimentos que a principal ainda nao tem (ela atrasa dias). A regra:
// tupla ja vista e abatida como repeticao; tupla inedita entra como
// movimento. Aqui, GASTOS CARTAO aparece nas duas tabelas e vale uma vez.
func TestAnalisarCSVIgnoraRecapitulacao(t *testing.T) {
	dados, err := os.ReadFile("testdata/sintetico_bradesco.csv")
	if err != nil {
		t.Fatalf("lendo fixture: %v", err)
	}

	transacoes, err := AnalisarCSV(dados, fusoDeTeste(t))
	if err != nil {
		t.Fatalf("AnalisarCSV: %v", err)
	}

	var vezes int
	for _, tr := range transacoes {
		if tr.Contraparte == "GASTOS CARTAO DE CREDITO" {
			vezes++
		}
	}
	if vezes != 1 {
		t.Fatalf("GASTOS CARTAO DE CREDITO apareceu %d vezes, quero 1: a recapitulacao foi parseada", vezes)
	}
}
