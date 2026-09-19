package extrato

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// Leitor do extrato CSV de conta corrente do Bradesco. Existe porque o
// Internet Banking do autor nao oferece OFX. Diferenca material para o OFX:
// nao ha identificador de transacao, entao a identidade nasce da impressao
// sha256 do payload, com um ordinal que desempata linhas identicas. O
// trade-off completo esta na spec 013.

var (
	ErrCabecalhoCSV = errors.New("extrato: cabecalho do CSV nao reconhecido")
	ErrLinhaCurta   = errors.New("extrato: linha do CSV com menos colunas que o cabecalho")
	ErrDoisValores  = errors.New("extrato: linha do CSV com credito e debito ao mesmo tempo")
)

// Colunas na ordem que o Bradesco exporta.
const (
	colData = iota
	colHistorico
	colDocumento
	colCredito
	colDebito
	colSaldo
	colunasMinimas
)

// AnalisarCSV le o extrato e devolve as transacoes na ordem do arquivo. O
// fuso vale para todas as datas: o CSV nao tem hora, entao cada lancamento
// fica a meia-noite local — o unico instante que o arquivo sustenta, e o que
// a competencia precisa.
func AnalisarCSV(dados []byte, fuso *time.Location) ([]Transacao, error) {
	if len(dados) > TamanhoMaximo {
		return nil, ErrArquivoGrande
	}

	// BOM de UTF-8: o Excel do banco emite, e sem remover o primeiro campo da
	// primeira linha viria com o marcador grudado.
	dados = bytes.TrimPrefix(dados, []byte{0xEF, 0xBB, 0xBF})

	leitor := csv.NewReader(bytes.NewReader(dados))
	leitor.Comma = ';'
	// O rodape ";;Total;...;" tem uma coluna a mais que o cabecalho; o numero
	// de campos e conferido linha a linha, nao pelo csv.Reader.
	leitor.FieldsPerRecord = -1
	leitor.LazyQuotes = true

	// Ordinais desempatam linhas identicas dentro do arquivo: sem isso, dois
	// PIX iguais no mesmo dia colapsariam num so pela constraint de impressao.
	ordinais := make(map[string]int)

	var transacoes []Transacao
	var viuCabecalho bool
	for linha := 1; ; linha++ {
		campos, err := leitor.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("extrato: linha %d: %w", linha, err)
		}

		if ehCabecalho(campos) {
			if viuCabecalho {
				// Segundo cabecalho: o Bradesco fecha o arquivo com uma secao
				// "Ultimos Lancamentos" que REPETE os movimentos finais da
				// tabela principal. Le-la duplicaria lancamentos reais — o
				// ordinal daria #1 as linhas repetidas, gerando impressao
				// nova e escapando da constraint. A primeira tabela e a
				// unica fonte.
				break
			}
			viuCabecalho = true
			continue
		}
		if ehDescartavel(campos) {
			continue
		}
		if len(campos) < colunasMinimas {
			return nil, fmt.Errorf("%w: linha %d", ErrLinhaCurta, linha)
		}

		t, err := lerLinhaCSV(campos, fuso, ordinais)
		if err != nil {
			// Posicao, nunca conteudo: a linha carrega valor e contraparte.
			if errors.Is(err, errLinhaIgnorada) {
				continue
			}
			return nil, fmt.Errorf("extrato: linha %d: %w", linha, err)
		}
		transacoes = append(transacoes, t)
	}

	if !viuCabecalho {
		return nil, ErrCabecalhoCSV
	}
	if len(transacoes) == 0 {
		return nil, ErrSemTransacoes
	}
	return transacoes, nil
}

// errLinhaIgnorada marca linha valida que nao vira lancamento (saldo
// anterior, total zerado). Sentinela interna: nao vaza do pacote.
var errLinhaIgnorada = errors.New("linha sem movimento")

// ehCabecalho reconhece a linha de titulos das colunas. Compara sem acento e
// sem caixa: o arquivo vem do Excel e ja mudou de acentuacao entre exports.
func ehCabecalho(campos []string) bool {
	if len(campos) < 3 {
		return false
	}
	primeiro := lancamento.Normalizar(campos[colData])
	segundo := lancamento.Normalizar(campos[colHistorico])
	// Normalizar devolve a forma canonica em MAIUSCULA, sem acento.
	return primeiro == "DATA" && strings.HasPrefix(segundo, "HISTORICO")
}

// ehDescartavel cobre o que nao e transacao. O criterio e a FORMA, nao o
// texto: uma linha de movimento tem data, historico e um valor — pelo menos
// dois campos preenchidos. Faixa do banco vem sempre como uma unica celula
// com a frase inteira dentro ("Extrato de: Ag ... | Conta ...", "Filtro de
// resultados - Movimentacao entre: ...", "Os dados acima tem como base ...",
// "Ultimos Lancamentos"). Casar por forma dispensa perseguir cada frase nova
// que o banco invente, e a primeira delas carrega agencia e conta, que nunca
// entram no payload.
func ehDescartavel(campos []string) bool {
	var preenchidos int
	for _, c := range campos {
		if strings.TrimSpace(c) != "" {
			preenchidos++
		}
	}
	if preenchidos <= 1 {
		return true
	}
	// Rodape: data vazia e a palavra Total na coluna do documento.
	if len(campos) > colDocumento &&
		strings.TrimSpace(campos[colData]) == "" &&
		lancamento.Normalizar(campos[colDocumento]) == "TOTAL" {
		return true
	}
	return false
}

func lerLinhaCSV(campos []string, fuso *time.Location, ordinais map[string]int) (Transacao, error) {
	data := strings.TrimSpace(campos[colData])
	historico := espacoSimples(campos[colHistorico])
	documento := strings.TrimSpace(campos[colDocumento])

	ocorridoEm, err := lerDataCSV(data, fuso)
	if err != nil {
		return Transacao{}, err
	}

	valor, err := lerValorDaLinha(campos[colCredito], campos[colDebito])
	if err != nil {
		return Transacao{}, err
	}
	// Saldo anterior e linhas informativas vem zeradas: evidencia sem fato.
	if valor == 0 {
		return Transacao{}, errLinhaIgnorada
	}

	if historico == "" {
		return Transacao{}, fmt.Errorf("%w: Historico", ErrCampoObrigatorio)
	}
	contraparte := historico
	if runas := []rune(contraparte); len(runas) > lancamento.ContraparteMaxima {
		contraparte = string(runas[:lancamento.ContraparteMaxima])
	}

	// Chave da tupla visivel; o ordinal conta repeticoes dela no arquivo.
	chave := strings.Join([]string{data, historico, documento, valor.String()}, "|")
	ordinal := ordinais[chave]
	ordinais[chave] = ordinal + 1

	// Payload guarda a linha reconstruida (sem cabecalho do banco, sem saldo)
	// e o ordinal, que e o que torna duas linhas identicas distinguiveis pela
	// impressao sha256 calculada em ocorrencia.Nova.
	payload := fmt.Sprintf("%s;%s;%s;%s;#%d", data, historico, documento, valor.String(), ordinal)

	return Transacao{
		OcorridoEm:  ocorridoEm,
		Valor:       valor,
		Meio:        meioDoHistorico(historico),
		Contraparte: contraparte,
		IDExterno:   "", // o CSV nao tem FITID; a identidade vive na impressao
		Payload:     payload,
	}, nil
}

// lerValorDaLinha resolve o sinal pela coluna, nao pelo numero: credito e
// positivo, debito e negativo. A coluna que nao vale vem com um espaco.
func lerValorDaLinha(creditoBruto, debitoBruto string) (dinheiro.Centavos, error) {
	credito := strings.TrimSpace(creditoBruto)
	debito := strings.TrimSpace(debitoBruto)

	switch {
	case credito != "" && debito != "":
		// Zerada dos dois lados e linha informativa, nao conflito.
		c, errC := lerValor(credito)
		d, errD := lerValor(debito)
		if errC == nil && errD == nil && c == 0 && d == 0 {
			return 0, nil
		}
		if errC == nil && c == 0 {
			v, err := lerValor(debito)
			return -v, err
		}
		if errD == nil && d == 0 {
			return lerValor(credito)
		}
		return 0, ErrDoisValores
	case credito != "":
		return lerValor(credito)
	case debito != "":
		v, err := lerValor(debito)
		if err != nil {
			return 0, err
		}
		return -v, nil
	default:
		return 0, nil
	}
}

// lerDataCSV le DD/MM/AAAA. O extrato nao traz hora: meia-noite no fuso do
// usuario, coerente com a competencia.
func lerDataCSV(texto string, fuso *time.Location) (time.Time, error) {
	if len(texto) != len("02/01/2006") {
		return time.Time{}, ErrDataInvalida
	}
	t, err := time.ParseInLocation("02/01/2006", texto, fuso)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", ErrDataInvalida, err)
	}
	return t, nil
}

// espacoSimples colapsa os espacos duplos que o banco emite
// ("PAGTO ELETRON  COBRANCA") para a contraparte normalizar igual sempre.
func espacoSimples(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// meioDoHistorico traduz o vocabulario real do extrato do Bradesco. A ordem
// do switch e a precedencia, e ela importa: "SAQUE CARTAO TRANSF PIX" casa
// com tres regras e deve ser PIX, que e o trilho por onde o dinheiro andou.
// Casamento por palavra inteira, nunca substring — "TED" mora dentro de
// LIMITED e "DOC" dentro de DOCERIA.
func meioDoHistorico(historico string) lancamento.Meio {
	palavras := strings.FieldsFunc(strings.ToUpper(historico), func(r rune) bool {
		return r < 'A' || r > 'Z'
	})
	tem := func(alvos ...string) bool {
		for _, p := range palavras {
			for _, alvo := range alvos {
				if p == alvo {
					return true
				}
			}
		}
		return false
	}
	temPrefixo := func(prefixos ...string) bool {
		for _, p := range palavras {
			for _, prefixo := range prefixos {
				if strings.HasPrefix(p, prefixo) {
					return true
				}
			}
		}
		return false
	}

	switch {
	case tem("PIX"):
		return lancamento.MeioPix
	case tem("SAQUE"):
		return lancamento.MeioDinheiro
	case tem("TED", "DOC") || temPrefixo("TRANSF"):
		return lancamento.MeioTransferencia
	case tem("BOLETO", "TITULO") || temPrefixo("COBRANC"):
		return lancamento.MeioBoleto
	case tem("CARTAO") && tem("CREDITO"):
		return lancamento.MeioCredito
	case temPrefixo("EMPRESTIMO", "PARCELA"):
		return lancamento.MeioCredito
	default:
		return lancamento.MeioDebito
	}
}
