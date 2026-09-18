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

var saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)

func TestAnalisarArquivoSintetico(t *testing.T) {
	dados, err := os.ReadFile("testdata/sintetico_basico.ofx")
	if err != nil {
		t.Fatal(err)
	}

	transacoes, err := Analisar(dados, saoPaulo)
	if err != nil {
		t.Fatalf("Analisar devolveu erro: %v", err)
	}
	if len(transacoes) != 3 {
		t.Fatalf("leu %d transacoes, queria 3", len(transacoes))
	}

	pix := transacoes[0]
	if pix.Valor != -4790 {
		t.Errorf("valor = %d, queria -4790", pix.Valor)
	}
	if pix.Meio != lancamento.MeioPix {
		t.Errorf("meio = %q, queria pix", pix.Meio)
	}
	if pix.IDExterno != "N0001" {
		t.Errorf("id externo = %q, queria N0001", pix.IDExterno)
	}
	if !strings.Contains(pix.Contraparte, "SUPERMERCADO SINTETICO") {
		t.Errorf("contraparte = %q", pix.Contraparte)
	}
	// 10:00 em -03:00 = 13:00 UTC.
	if utc := pix.OcorridoEm.UTC(); utc.Hour() != 13 || utc.Day() != 2 {
		t.Errorf("ocorrido em %v, queria 2026-09-02 13:00 UTC", utc)
	}
	if !strings.Contains(pix.Payload, "<TRNAMT>-47.90") {
		t.Errorf("payload nao carrega o bloco bruto: %q", pix.Payload)
	}

	if ted := transacoes[1]; ted.Valor != 350000 || ted.Meio != lancamento.MeioTransferencia {
		t.Errorf("TED = (%d, %s), queria (350000, transferencia)", ted.Valor, ted.Meio)
	}
	if boleto := transacoes[2]; boleto.Valor != -123456 || boleto.Meio != lancamento.MeioBoleto {
		t.Errorf("boleto = (%d, %s), queria (-123456, boleto)", boleto.Valor, boleto.Meio)
	}
}

func TestAnalisarLatin1(t *testing.T) {
	// "AÇAÍ" em Windows-1252: C7 = Ç, CD = Í. Bytes crus, sem UTF-8.
	ofx := "OFXHEADER:100\nENCODING:USASCII\nCHARSET:1252\n\n<OFX><STMTTRN>" +
		"<DTPOSTED>20260901\n<TRNAMT>-10.00\n<FITID>X1\n<MEMO>A\xc7A\xcd DO CENTRO\n" +
		"</STMTTRN></OFX>"

	transacoes, err := Analisar([]byte(ofx), saoPaulo)
	if err != nil {
		t.Fatalf("Analisar devolveu erro: %v", err)
	}
	if got := transacoes[0].Contraparte; got != "AÇAÍ DO CENTRO" {
		t.Errorf("contraparte = %q, queria AÇAÍ DO CENTRO", got)
	}
}

func TestAnalisarErro(t *testing.T) {
	casos := []struct {
		nome string
		ofx  string
		erro error
	}{
		{"vazio", "", ErrSemTransacoes},
		{"sem transacao", "<OFX><BANKTRANLIST></BANKTRANLIST></OFX>", ErrSemTransacoes},
		{"sem FITID", "<OFX><STMTTRN><DTPOSTED>20260901\n<TRNAMT>-1.00\n<MEMO>X</STMTTRN></OFX>", ErrCampoObrigatorio},
		{"sem valor", "<OFX><STMTTRN><DTPOSTED>20260901\n<FITID>A\n<MEMO>X</STMTTRN></OFX>", ErrCampoObrigatorio},
		{"sem memo e name", "<OFX><STMTTRN><DTPOSTED>20260901\n<TRNAMT>-1.00\n<FITID>A</STMTTRN></OFX>", ErrCampoObrigatorio},
		{"data curta", "<OFX><STMTTRN><DTPOSTED>2026\n<TRNAMT>-1.00\n<FITID>A\n<MEMO>X</STMTTRN></OFX>", ErrDataInvalida},
		{"fuso sem fechar", "<OFX><STMTTRN><DTPOSTED>20260901[-3\n<TRNAMT>-1.00\n<FITID>A\n<MEMO>X</STMTTRN></OFX>", ErrDataInvalida},
		{"valor com letra", "<OFX><STMTTRN><DTPOSTED>20260901\n<TRNAMT>-1a.00\n<FITID>A\n<MEMO>X</STMTTRN></OFX>", ErrValorInvalido},
		{"valor so com sinal", "<OFX><STMTTRN><DTPOSTED>20260901\n<TRNAMT>-\n<FITID>A\n<MEMO>X</STMTTRN></OFX>", ErrValorInvalido},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := Analisar([]byte(c.ofx), saoPaulo); !errors.Is(err, c.erro) {
				t.Errorf("Analisar devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

func TestAnalisarArquivoGrande(t *testing.T) {
	if _, err := Analisar(make([]byte, TamanhoMaximo+1), saoPaulo); !errors.Is(err, ErrArquivoGrande) {
		t.Errorf("devolveu %v, queria ErrArquivoGrande", err)
	}
}

func TestLerValor(t *testing.T) {
	casos := []struct {
		entrada string
		querido dinheiro.Centavos
		erro    bool
	}{
		{"-47.90", -4790, false},
		{"-47,90", -4790, false},
		{"3500.00", 350000, false},
		{"-1234.56", -123456, false},
		{"-1.234,56", -123456, false},
		{"1,234.56", 123456, false},
		{"-47.9", -4790, false}, // uma casa = decimos
		{"10", 1000, false},
		{"+0.01", 1, false},
		{"1.234", 123400, false}, // ponto com 3 digitos = milhar
		{"1,234", 0, true},       // virgula com 3 digitos nao existe em dinheiro
		{"", 0, true},
		{"-", 0, true},
		{"abc", 0, true},
		{"1.2.3", 0, true},
	}

	for _, c := range casos {
		t.Run(c.entrada, func(t *testing.T) {
			obtido, err := lerValor(c.entrada)
			if (err != nil) != c.erro {
				t.Fatalf("lerValor(%q) erro = %v, esperava erro = %v", c.entrada, err, c.erro)
			}
			if obtido != c.querido {
				t.Errorf("lerValor(%q) = %d, queria %d", c.entrada, obtido, c.querido)
			}
		})
	}
}

func TestLerData(t *testing.T) {
	utc3 := time.Date(2026, 9, 30, 22, 30, 0, 0, time.FixedZone("UTC-3", -3*3600))

	casos := []struct {
		nome    string
		entrada string
		querida time.Time
	}{
		{"so data no fuso padrao", "20260930", time.Date(2026, 9, 30, 0, 0, 0, 0, saoPaulo)},
		{"data e hora com fuso", "20260930223000[-3:BRT]", utc3},
		{"fuso com dois digitos", "20260930223000[-03:BRT]", utc3},
		{"fracao de segundo", "20260930223000.000[-3:BRT]", utc3},
		{"gmt zero", "20260930120000[0:GMT]", time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			obtida, err := lerData(c.entrada, saoPaulo)
			if err != nil {
				t.Fatalf("lerData(%q) erro: %v", c.entrada, err)
			}
			if !obtida.Equal(c.querida) {
				t.Errorf("lerData(%q) = %v, queria %v", c.entrada, obtida, c.querida)
			}
		})
	}
}

func TestMeioDaTransacao(t *testing.T) {
	casos := []struct {
		texto   string
		querido lancamento.Meio
	}{
		{"PIX QR CODE DINAMICO", lancamento.MeioPix},
		{"CRED TED 102", lancamento.MeioTransferencia},
		{"TRANSF SALDO CC", lancamento.MeioTransferencia},
		{"PAGTO BOLETO TITULO", lancamento.MeioBoleto},
		{"SAQUE 24H", lancamento.MeioDinheiro},
		{"COMPRA VISA ELECTRON", lancamento.MeioDebito},
		{"", lancamento.MeioDebito},
	}

	for _, c := range casos {
		if obtido := meioDaTransacao(c.texto); obtido != c.querido {
			t.Errorf("meioDaTransacao(%q) = %s, queria %s", c.texto, obtido, c.querido)
		}
	}
}

// FuzzAnalisar: entrada arbitraria nunca causa panic, e toda transacao aceita
// tem os invariantes minimos (valor cabe no dominio, id externo presente).
func FuzzAnalisar(f *testing.F) {
	if dados, err := os.ReadFile("testdata/sintetico_basico.ofx"); err == nil {
		f.Add(dados)
	}
	f.Add([]byte("<OFX><STMTTRN><DTPOSTED>20260901\n<TRNAMT>-1,5\n<FITID>A\n<MEMO>X</STMTTRN>"))
	f.Add([]byte("OFXHEADER:100\n\n<OFX>"))
	f.Add([]byte("<STMTTRN><STMTTRN><STMTTRN>"))

	f.Fuzz(func(t *testing.T, dados []byte) {
		transacoes, err := Analisar(dados, saoPaulo)
		if err != nil {
			return
		}
		for _, tr := range transacoes {
			if tr.IDExterno == "" {
				t.Error("transacao aceita sem FITID")
			}
			if tr.Contraparte == "" {
				t.Error("transacao aceita sem contraparte")
			}
			if tr.OcorridoEm.IsZero() {
				t.Error("transacao aceita com data zero")
			}
		}
	})
}
