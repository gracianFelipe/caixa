package bradesco

import (
	"errors"
	"strings"
	"testing"

	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// eml monta um .eml sintetico de cabecalhos + corpo.
func eml(cabecalhos map[string]string, corpo string) []byte {
	var b strings.Builder
	base := map[string]string{
		"From":       "Bradesco <alerta@infobradesco.com.br>",
		"To":         "dono@example.com",
		"Subject":    "Bradesco: movimentacao na sua conta",
		"Date":       "Thu, 17 Sep 2026 15:04:05 -0300",
		"Message-Id": "<abc123@infobradesco.com.br>",
	}
	for k, v := range cabecalhos {
		base[k] = v
	}
	for k, v := range base {
		if v != "" {
			b.WriteString(k + ": " + v + "\r\n")
		}
	}
	b.WriteString("\r\n")
	b.WriteString(corpo)
	return []byte(b.String())
}

func TestAnalisarTextoPlano(t *testing.T) {
	bruto := eml(nil, "Compra aprovada no cartao de debito.\r\nValor: R$ 47,90\r\nEstabelecimento: SUPERMERCADO XYZ LTDA\r\nData: 17/09/2026 15:04\r\n")

	tr, err := Analisar(bruto)
	if err != nil {
		t.Fatalf("Analisar: %v", err)
	}
	if tr.Valor != -4790 {
		t.Errorf("valor = %d, queria -4790 (saida)", tr.Valor)
	}
	if tr.Contraparte != "SUPERMERCADO XYZ LTDA" {
		t.Errorf("contraparte = %q", tr.Contraparte)
	}
	if tr.IDExterno != "abc123@infobradesco.com.br" {
		t.Errorf("id externo = %q", tr.IDExterno)
	}
	if tr.OcorridoEm.UTC().Hour() != 18 { // 15:04 -0300 = 18:04 UTC
		t.Errorf("ocorrido em %v", tr.OcorridoEm.UTC())
	}
	if tr.Meio != lancamento.MeioDebito {
		t.Errorf("meio = %s", tr.Meio)
	}
	if strings.Contains(tr.Payload, "\r") || strings.Contains(tr.Payload, "\n") {
		t.Error("payload deveria ter espacos colapsados")
	}
}

func TestAnalisarQuotedPrintableLatin1(t *testing.T) {
	corpo := "Pix enviado.\r\nValor: R$ 1.234,56\r\nFavorecido: A=C7A=CD DO CENTRO\r\n"
	bruto := eml(map[string]string{
		"Content-Type":              `text/plain; charset="iso-8859-1"`,
		"Content-Transfer-Encoding": "quoted-printable",
	}, corpo)

	tr, err := Analisar(bruto)
	if err != nil {
		t.Fatalf("Analisar: %v", err)
	}
	if tr.Valor != -123456 {
		t.Errorf("valor = %d", tr.Valor)
	}
	if tr.Contraparte != "AÇAÍ DO CENTRO" {
		t.Errorf("contraparte = %q", tr.Contraparte)
	}
	if tr.Meio != lancamento.MeioPix {
		t.Errorf("meio = %s, queria pix", tr.Meio)
	}
}

func TestAnalisarMultipartComHTML(t *testing.T) {
	corpo := "--fronteira\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<html><body><p>Compra no cart&atilde;o de cr&eacute;dito CRED</p>" +
		"<table><tr><td>Valor:</td><td>R$ 250,00</td></tr>" +
		"<tr><td>Local:</td><td>ELETRO LOJA SA</td></tr></table></body></html>\r\n" +
		"--fronteira--\r\n"
	bruto := eml(map[string]string{
		"Content-Type": `multipart/alternative; boundary="fronteira"`,
	}, corpo)

	tr, err := Analisar(bruto)
	if err != nil {
		t.Fatalf("Analisar: %v", err)
	}
	if tr.Valor != -25000 {
		t.Errorf("valor = %d", tr.Valor)
	}
	if !strings.HasPrefix(tr.Contraparte, "ELETRO LOJA") {
		t.Errorf("contraparte = %q", tr.Contraparte)
	}
	if tr.Meio != lancamento.MeioCredito {
		t.Errorf("meio = %s, queria credito", tr.Meio)
	}
}

func TestAnalisarPreferePlanoSobreHTML(t *testing.T) {
	corpo := "--fronteira\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Valor: R$ 10,00\r\nLocal: DO TEXTO PLANO\r\n\r\n" +
		"--fronteira\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<p>Valor: R$ 99,99</p><p>Local: DO HTML</p>\r\n" +
		"--fronteira--\r\n"
	bruto := eml(map[string]string{
		"Content-Type": `multipart/alternative; boundary="fronteira"`,
	}, corpo)

	tr, err := Analisar(bruto)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Valor != -1000 || tr.Contraparte != "DO TEXTO PLANO" {
		t.Errorf("deveria preferir text/plain: (%d, %q)", tr.Valor, tr.Contraparte)
	}
}

func TestAnalisarErro(t *testing.T) {
	casos := []struct {
		nome  string
		bruto []byte
		erro  error
	}{
		{"sem valor", eml(nil, "Sua fatura fechou. Acesse o app.\r\n"), ErrSemValor},
		{"valor sem contraparte", eml(nil, "Valor: R$ 10,00\r\nSem mais detalhes.\r\n"), ErrSemContraparte},
		{"sem date", eml(map[string]string{"Date": ""}, "Valor: R$ 10,00\r\nLocal: X\r\n"), ErrSemData},
		{"valor sem centavos nao casa", eml(nil, "Valor: R$ 10\r\nLocal: X\r\n"), ErrSemValor},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := Analisar(c.bruto); !errors.Is(err, c.erro) {
				t.Errorf("Analisar = %v, queria %v", err, c.erro)
			}
		})
	}
}
