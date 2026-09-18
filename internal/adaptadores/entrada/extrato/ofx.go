// Package extrato le arquivos OFX 1.x de banco brasileiro. O formato e SGML,
// nao XML: tags de folha nao fecham (<TRNAMT>-47.90 e uma linha completa),
// entao encoding/xml nao serve e o tokenizador e proprio — tolerante com a
// sopa de tags, estrito com os valores que viram dinheiro e data.
package extrato

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// TamanhoMaximo limita o arquivo aceito: um OFX de 6 meses tem dezenas de KiB;
// 8 MiB e teto generoso, nao permissao para engolir qualquer coisa.
const TamanhoMaximo = 8 << 20

var (
	ErrArquivoGrande    = errors.New("extrato: arquivo acima de 8 MiB")
	ErrSemTransacoes    = errors.New("extrato: nenhum bloco STMTTRN encontrado")
	ErrValorInvalido    = errors.New("extrato: valor de transacao invalido")
	ErrDataInvalida     = errors.New("extrato: data de transacao invalida")
	ErrCampoObrigatorio = errors.New("extrato: transacao sem campo obrigatorio")
)

// Transacao e uma linha do extrato ja tipada, pronta para o cmd converter no
// DTO da aplicacao. Payload guarda o bloco STMTTRN bruto (texto decodificado,
// espacos colapsados): e dele que nasce a impressao de idempotencia.
type Transacao struct {
	OcorridoEm  time.Time
	Valor       dinheiro.Centavos
	Meio        lancamento.Meio
	Contraparte string
	IDExterno   string
	Payload     string
}

// Analisar decodifica o arquivo e devolve as transacoes na ordem do extrato.
// O fusoPadrao vale para datas sem [gmt:tz] explicito no DTPOSTED.
func Analisar(dados []byte, fusoPadrao *time.Location) ([]Transacao, error) {
	if len(dados) > TamanhoMaximo {
		return nil, ErrArquivoGrande
	}

	texto, err := decodificar(dados)
	if err != nil {
		return nil, err
	}

	blocos := blocosSTMTTRN(texto)
	if len(blocos) == 0 {
		return nil, ErrSemTransacoes
	}

	transacoes := make([]Transacao, 0, len(blocos))
	for i, bloco := range blocos {
		t, err := lerTransacao(bloco, fusoPadrao)
		if err != nil {
			// Posicao no arquivo, nunca o conteudo: a linha carrega valor e
			// contraparte, que nao entram em erro nem em log.
			return nil, fmt.Errorf("transacao %d: %w", i+1, err)
		}
		transacoes = append(transacoes, t)
	}
	return transacoes, nil
}

// decodificar separa o cabecalho OFX (linhas CHAVE:VALOR, sempre ASCII) e
// converte o corpo para UTF-8. Banco brasileiro emite Windows-1252/latin-1;
// so se o cabecalho declarar UTF-8 os bytes passam direto. Windows-1252 e
// superconjunto do ASCII, entao decodificar um arquivo ASCII por ele e no-op.
func decodificar(dados []byte) (string, error) {
	cabecalho := dados
	if i := indexByte(dados, '<'); i >= 0 {
		cabecalho = dados[:i]
	}
	declaraUTF8 := strings.Contains(strings.ToUpper(string(cabecalho)), "UTF-8")

	if declaraUTF8 {
		return string(dados), nil
	}
	texto, err := charmap.Windows1252.NewDecoder().Bytes(dados)
	if err != nil {
		return "", fmt.Errorf("extrato: decodificando windows-1252: %w", err)
	}
	return string(texto), nil
}

func indexByte(b []byte, alvo byte) int {
	for i := range b {
		if b[i] == alvo {
			return i
		}
	}
	return -1
}

// blocosSTMTTRN recorta cada <STMTTRN>...</STMTTRN>. Se um arquivo nao fechar
// o bloco, o proximo <STMTTRN> (ou o fim do arquivo) encerra o anterior.
// Comparacao de tag com EqualFold, nunca ToUpper no texto inteiro: ha
// caracteres que mudam de tamanho em maiuscula e desalinhariam os offsets.
func blocosSTMTTRN(texto string) []string {
	var blocos []string
	inicio := -1 // posicao do conteudo do bloco aberto, ou -1
	pos := 0
	for pos < len(texto) {
		abre := strings.IndexByte(texto[pos:], '<')
		if abre < 0 {
			break
		}
		abre += pos
		fecha := strings.IndexByte(texto[abre:], '>')
		if fecha < 0 {
			break
		}
		nome := strings.TrimSpace(texto[abre+1 : abre+fecha])
		pos = abre + fecha + 1

		switch {
		case strings.EqualFold(nome, "STMTTRN"):
			if inicio >= 0 {
				blocos = append(blocos, texto[inicio:abre])
			}
			inicio = pos
		case strings.EqualFold(nome, "/STMTTRN"):
			if inicio >= 0 {
				blocos = append(blocos, texto[inicio:abre])
				inicio = -1
			}
		}
	}
	if inicio >= 0 {
		blocos = append(blocos, texto[inicio:])
	}
	return blocos
}

// camposDoBloco tokeniza a sopa de tags de um bloco: cada <TAG> abre uma
// folha e o texto ate o proximo '<' e o valor dela. Tag repetida: a primeira
// vale (OFX nao repete folha dentro de STMTTRN; repeticao e lixo).
func camposDoBloco(bloco string) map[string]string {
	campos := make(map[string]string)
	resto := bloco
	for {
		abre := strings.IndexByte(resto, '<')
		if abre < 0 {
			break
		}
		fechaTag := strings.IndexByte(resto[abre:], '>')
		if fechaTag < 0 {
			break
		}
		nome := strings.ToUpper(strings.TrimSpace(resto[abre+1 : abre+fechaTag]))
		resto = resto[abre+fechaTag+1:]

		fimValor := strings.IndexByte(resto, '<')
		if fimValor < 0 {
			fimValor = len(resto)
		}
		valor := desescapar(strings.TrimSpace(resto[:fimValor]))

		if nome != "" && !strings.HasPrefix(nome, "/") && valor != "" {
			if _, existe := campos[nome]; !existe {
				campos[nome] = valor
			}
		}
	}
	return campos
}

// desescapar cobre as entidades que aparecem em OFX de banco; nao e um parser
// de HTML e nao precisa ser.
var desescapador = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">",
	"&quot;", `"`, "&apos;", "'", "&nbsp;", " ",
)

func desescapar(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	return desescapador.Replace(s)
}

func lerTransacao(bloco string, fusoPadrao *time.Location) (Transacao, error) {
	campos := camposDoBloco(bloco)

	for _, obrigatorio := range []string{"DTPOSTED", "TRNAMT", "FITID"} {
		if campos[obrigatorio] == "" {
			return Transacao{}, fmt.Errorf("%w: %s", ErrCampoObrigatorio, obrigatorio)
		}
	}

	ocorridoEm, err := lerData(campos["DTPOSTED"], fusoPadrao)
	if err != nil {
		return Transacao{}, err
	}
	valor, err := lerValor(campos["TRNAMT"])
	if err != nil {
		return Transacao{}, err
	}

	// NAME identifica a contraparte quando existe; MEMO e a descricao livre.
	contraparte := campos["NAME"]
	if contraparte == "" {
		contraparte = campos["MEMO"]
	}
	if contraparte == "" {
		return Transacao{}, fmt.Errorf("%w: NAME ou MEMO", ErrCampoObrigatorio)
	}
	if runas := []rune(contraparte); len(runas) > lancamento.ContraparteMaxima {
		contraparte = string(runas[:lancamento.ContraparteMaxima])
	}

	return Transacao{
		OcorridoEm:  ocorridoEm,
		Valor:       valor,
		Meio:        meioDaTransacao(campos["MEMO"] + " " + campos["NAME"]),
		Contraparte: contraparte,
		IDExterno:   campos["FITID"],
		Payload:     strings.Join(strings.Fields(bloco), " "),
	}, nil
}

// lerValor converte o TRNAMT em centavos sem passar por float. Aceita ponto
// OU virgula como decimal (1-2 casas); com os dois separadores presentes, o
// ultimo e o decimal e o outro e milhar ("-1.234,56", "1,234.56"). Ponto com
// tres digitos e milhar ("1.234" = 1234,00): tres casas decimais nao existem
// em dinheiro. Agrupamento de milhar errado e recusado, nao adivinhado.
func lerValor(texto string) (dinheiro.Centavos, error) {
	s := strings.TrimSpace(texto)
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if s == "" {
		return 0, ErrValorInvalido
	}

	ultimoPonto, ultimaVirgula := strings.LastIndexByte(s, '.'), strings.LastIndexByte(s, ',')
	inteiro, decimal := s, ""
	switch {
	case ultimoPonto >= 0 && ultimaVirgula >= 0:
		corte := max(ultimoPonto, ultimaVirgula)
		inteiro, decimal = s[:corte], s[corte+1:]
	case ultimoPonto >= 0 || ultimaVirgula >= 0:
		corte := max(ultimoPonto, ultimaVirgula)
		casas := len(s) - corte - 1
		switch {
		case casas >= 1 && casas <= 2:
			inteiro, decimal = s[:corte], s[corte+1:]
		case casas == 3 && s[corte] == '.':
			// milhar sem decimais; a validacao de grupos abaixo confere
		default:
			return 0, ErrValorInvalido
		}
	}
	if len(decimal) > 2 {
		return 0, ErrValorInvalido
	}

	// Valida os grupos de milhar da parte inteira: "1.2" e "12.3456" sao
	// erro de digitacao disfarcado, nao dinheiro.
	grupos := strings.Split(strings.ReplaceAll(inteiro, ",", "."), ".")
	for i, g := range grupos {
		if g == "" {
			return 0, ErrValorInvalido
		}
		for j := 0; j < len(g); j++ {
			if g[j] < '0' || g[j] > '9' {
				return 0, ErrValorInvalido
			}
		}
		if i > 0 && len(g) != 3 {
			return 0, ErrValorInvalido
		}
		if i == 0 && len(grupos) > 1 && len(g) > 3 {
			return 0, ErrValorInvalido
		}
	}

	switch len(decimal) {
	case 0:
		decimal = "00"
	case 1:
		decimal += "0" // "-47.9" e valido em OFX: uma casa = decimos
	}

	valor, err := dinheiro.Analisar(strings.Join(grupos, "") + "," + decimal)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrValorInvalido, err)
	}
	if negativo {
		valor = -valor
	}
	return valor, nil
}

// lerData le AAAAMMDD, opcionalmente com HHMMSS, fracao ".XXX" e fuso
// "[−3:BRT]". Sem fuso explicito vale o fusoPadrao — a mesma decisao da
// competencia: o instante pertence ao dia no fuso do usuario.
func lerData(texto string, fusoPadrao *time.Location) (time.Time, error) {
	s := strings.TrimSpace(texto)
	fuso := fusoPadrao

	if i := strings.IndexByte(s, '['); i >= 0 {
		fim := strings.IndexByte(s[i:], ']')
		if fim < 0 {
			return time.Time{}, ErrDataInvalida
		}
		deslocamento := s[i+1 : i+fim]
		s = s[:i]
		if dois := strings.IndexByte(deslocamento, ':'); dois >= 0 {
			deslocamento = deslocamento[:dois] // "[-3:BRT]" -> "-3"
		}
		horas, err := strconv.Atoi(strings.TrimSpace(deslocamento))
		if err != nil || horas < -12 || horas > 14 {
			return time.Time{}, ErrDataInvalida
		}
		fuso = time.FixedZone(fmt.Sprintf("UTC%+d", horas), horas*60*60)
	}

	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i] // fracao de segundo nao interessa a um extrato
	}

	var formato string
	switch len(s) {
	case 8:
		formato = "20060102"
	case 14:
		formato = "20060102150405"
	default:
		return time.Time{}, ErrDataInvalida
	}

	t, err := time.ParseInLocation(formato, s, fuso)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", ErrDataInvalida, err)
	}
	return t, nil
}

// meioDaTransacao adivinha o meio a partir do texto livre do extrato.
// Heuristica assumida e ajustavel: quando o OFX real do Bradesco chegar,
// cada acerto/erro vira caso de teste. Na duvida, debito — e o meio mais
// comum em conta corrente e o menos enganoso quando errado.
func meioDaTransacao(texto string) lancamento.Meio {
	t := strings.ToUpper(texto)
	switch {
	case strings.Contains(t, "PIX"):
		return lancamento.MeioPix
	case strings.Contains(t, "TED") || strings.Contains(t, "DOC") || strings.Contains(t, "TRANSF"):
		return lancamento.MeioTransferencia
	case strings.Contains(t, "BOLETO") || strings.Contains(t, "TITULO"):
		return lancamento.MeioBoleto
	case strings.Contains(t, "SAQUE"):
		return lancamento.MeioDinheiro
	default:
		return lancamento.MeioDebito
	}
}
