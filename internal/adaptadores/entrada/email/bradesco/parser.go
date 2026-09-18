// Package bradesco extrai um movimento financeiro do e-mail de alerta do
// banco. Parsing 100% stdlib sobre o .eml cru: net/mail para a estrutura,
// mime/* para as partes, e rotulos tolerantes para valor e contraparte.
//
// PREMISSA NAO VERIFICADA (risco n. 1 do plano): o formato real do alerta.
// As fixtures sao sinteticas; com os primeiros .eml reais, cada divergencia
// vira caso de teste aqui.
package bradesco

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

var (
	ErrSemValor       = errors.New("bradesco: e-mail sem valor identificavel")
	ErrSemContraparte = errors.New("bradesco: e-mail sem contraparte identificavel")
	ErrSemData        = errors.New("bradesco: e-mail sem cabecalho Date valido")
)

// Transacao e o que o alerta afirma. Payload guarda o texto extraido (nao o
// .eml inteiro): e a base da impressao de idempotencia e cabe no JSONB.
type Transacao struct {
	OcorridoEm  time.Time
	Valor       dinheiro.Centavos
	Meio        lancamento.Meio
	Contraparte string
	IDExterno   string // Message-Id
	Payload     string
}

// tamanho maximo de corpo que se processa: alerta de banco tem poucos KB.
const corpoMaximo = 1 << 20

// Analisar le o .eml cru e extrai a transacao.
func Analisar(bruto []byte) (Transacao, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(bruto))
	if err != nil {
		return Transacao{}, fmt.Errorf("bradesco: lendo mensagem: %w", err)
	}

	quando, err := msg.Header.Date()
	if err != nil {
		return Transacao{}, ErrSemData
	}

	texto, err := textoDoCorpo(msg)
	if err != nil {
		return Transacao{}, err
	}

	valor, err := extrairValor(texto)
	if err != nil {
		return Transacao{}, err
	}
	contraparte, err := extrairContraparte(texto)
	if err != nil {
		return Transacao{}, err
	}

	return Transacao{
		OcorridoEm:  quando,
		Valor:       -valor, // alerta de movimentacao e gasto: saida
		Meio:        meioDoTexto(texto),
		Contraparte: contraparte,
		IDExterno:   strings.Trim(msg.Header.Get("Message-Id"), "<> "),
		Payload:     strings.Join(strings.Fields(texto), " "),
	}, nil
}

// --- corpo: multipart, transfer-encoding e charset -------------------------

// textoDoCorpo prefere text/plain; sem ele, decapa o HTML. Cobre multipart
// aninhado, quoted-printable, base64 e latin-1/1252.
func textoDoCorpo(msg *mail.Message) (string, error) {
	tipo := msg.Header.Get("Content-Type")
	if tipo == "" {
		tipo = "text/plain"
	}
	texto, html, err := coletar(msg.Body, tipo, msg.Header.Get("Content-Transfer-Encoding"), 0)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(texto) != "" {
		return texto, nil
	}
	if strings.TrimSpace(html) != "" {
		return decaparHTML(html), nil
	}
	return "", ErrSemValor
}

func coletar(corpo io.Reader, contentType, encoding string, nivel int) (texto, html string, err error) {
	if nivel > 5 {
		return "", "", errors.New("bradesco: multipart aninhado demais")
	}

	tipo, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		tipo = "text/plain"
	}

	if strings.HasPrefix(tipo, "multipart/") {
		leitor := multipart.NewReader(corpo, params["boundary"])
		for {
			parte, err := leitor.NextPart()
			if errors.Is(err, io.EOF) {
				return texto, html, nil
			}
			if err != nil {
				return texto, html, nil // parte truncada: usa o que ja veio
			}
			t, h, err := coletar(parte,
				parte.Header.Get("Content-Type"),
				parte.Header.Get("Content-Transfer-Encoding"), nivel+1)
			if err != nil {
				return "", "", err
			}
			texto += t
			html += h
		}
	}

	conteudo, err := lerDecodificado(corpo, encoding, params["charset"])
	if err != nil {
		return "", "", err
	}
	switch tipo {
	case "text/plain":
		return conteudo, "", nil
	case "text/html":
		return "", conteudo, nil
	default:
		return "", "", nil // anexo: ignora
	}
}

func lerDecodificado(r io.Reader, encoding, charset string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, r)
	}
	bruto, err := io.ReadAll(io.LimitReader(r, corpoMaximo))
	if err != nil {
		return "", fmt.Errorf("bradesco: lendo corpo: %w", err)
	}

	switch strings.ToLower(charset) {
	case "iso-8859-1", "latin1", "windows-1252", "cp1252":
		decodificado, err := charmap.Windows1252.NewDecoder().Bytes(bruto)
		if err != nil {
			return "", fmt.Errorf("bradesco: charset: %w", err)
		}
		return string(decodificado), nil
	default:
		return string(bruto), nil
	}
}

var (
	tagsDeQuebra = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/tr|/li|/h[1-6])[^>]*>`)
	qualquerTag  = regexp.MustCompile(`<[^>]*>`)
)

// decaparHTML vira texto: quebras estruturais viram \n, o resto das tags cai.
// Nao e um parser de HTML — e o minimo para regex de rotulo funcionar.
func decaparHTML(html string) string {
	s := tagsDeQuebra.ReplaceAllString(html, "\n")
	s = qualquerTag.ReplaceAllString(s, " ")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`).Replace(s)
	return s
}

// --- extracao por rotulo ----------------------------------------------------

// reValor casa "R$ 47,90", "R$1.234,56" — o formato brasileiro que o
// dominio ja sabe ler; o parser so recorta.
var reValor = regexp.MustCompile(`R\$\s*([0-9][0-9\.]*,[0-9]{2})`)

func extrairValor(texto string) (dinheiro.Centavos, error) {
	m := reValor.FindStringSubmatch(texto)
	if m == nil {
		return 0, ErrSemValor
	}
	v, err := dinheiro.Analisar(m[1])
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrSemValor, err)
	}
	return v, nil
}

// rotulosDeContraparte na ordem de preferencia observavel nos alertas.
// O fallback "em MAIUSCULAS" e case-SENSITIVE de proposito: com (?i) a
// classe A-Z casaria minusculas e "S(em m)ais detalhes" viraria contraparte.
var rotulosDeContraparte = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:estabelecimento|local|favorecido|para|destinatario)\s*:\s*([^\n\r]+)`),
	regexp.MustCompile(`(?:^|\s)em\s+([A-ZÀ-Ú][A-ZÀ-Ú0-9 \.\*&-]{3,60})`),
}

func extrairContraparte(texto string) (string, error) {
	for _, re := range rotulosDeContraparte {
		if m := re.FindStringSubmatch(texto); m != nil {
			c := strings.TrimSpace(m[1])
			if c != "" {
				return c, nil
			}
		}
	}
	return "", ErrSemContraparte
}

func meioDoTexto(texto string) lancamento.Meio {
	t := strings.ToUpper(texto)
	switch {
	case strings.Contains(t, "PIX"):
		return lancamento.MeioPix
	case strings.Contains(t, "CART") && strings.Contains(t, "CR"):
		return lancamento.MeioCredito
	case strings.Contains(t, "BOLETO"):
		return lancamento.MeioBoleto
	default:
		return lancamento.MeioDebito
	}
}
