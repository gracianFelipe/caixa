// Package dinheiro representa valor monetario como quantidade inteira de
// centavos. Nao existe ponto flutuante em nenhum ponto deste pacote.
package dinheiro

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Centavos e a unidade monetaria do sistema: 1 real = 100 Centavos.
// Negativo significa saida de dinheiro.
type Centavos int64

// Erros sentinela: o chamador compara com errors.Is, nunca com a mensagem.
var (
	ErrVazio   = errors.New("dinheiro: texto vazio")
	ErrFormato = errors.New("dinheiro: formato invalido")
	ErrEstouro = errors.New("dinheiro: valor fora do alcance de int64")
)

// Analisar converte a escrita monetaria brasileira em Centavos.
// Aceita "R$ 1.234,56", "1.234,56", "1234,56", "-R$ 10,00" e "R$ -10,00".
// O ponto e separador de milhar e a virgula e separador decimal.
func Analisar(texto string) (Centavos, error) {
	s := strings.TrimSpace(texto)
	if s == "" {
		return 0, ErrVazio
	}

	// O sinal pode vir antes ou depois do simbolo, nunca nos dois lugares.
	negativo, s, err := extrairSinal(s)
	if err != nil {
		return 0, err
	}

	inteiro, decimal, temVirgula := strings.Cut(s, ",")
	if strings.ContainsRune(decimal, ',') {
		return 0, ErrFormato
	}
	if !temVirgula {
		decimal = "00"
	} else if len(decimal) != 2 {
		// "R$ 1,5" e ambiguo: 5 centavos ou 50? Recusar e mais seguro que adivinhar.
		return 0, ErrFormato
	}

	digitos, err := digitosDaParteInteira(inteiro)
	if err != nil {
		return 0, err
	}

	total, err := acumular(digitos + decimal)
	if err != nil {
		return 0, err
	}
	if negativo {
		return -total, nil
	}
	return total, nil
}

// extrairSinal remove o sinal e o simbolo "R$", devolvendo o resto do texto.
func extrairSinal(s string) (negativo bool, resto string, err error) {
	if r, ok := strings.CutPrefix(s, "-"); ok {
		negativo, s = true, strings.TrimSpace(r)
	} else if r, ok := strings.CutPrefix(s, "+"); ok {
		s = strings.TrimSpace(r)
	}

	if r, ok := strings.CutPrefix(s, "R$"); ok {
		s = strings.TrimSpace(r)
	}

	if r, ok := strings.CutPrefix(s, "-"); ok {
		if negativo {
			return false, "", ErrFormato // "-R$ -10,00"
		}
		negativo, s = true, strings.TrimSpace(r)
	} else if r, ok := strings.CutPrefix(s, "+"); ok {
		if negativo {
			return false, "", ErrFormato // "-R$ +10,00"
		}
		s = strings.TrimSpace(r)
	}

	return negativo, s, nil
}

// digitosDaParteInteira valida o agrupamento de milhar e devolve so os digitos.
// "1.234" vira "1234"; "1.2345" e recusado, porque e erro de digitacao disfarcado.
func digitosDaParteInteira(inteiro string) (string, error) {
	grupos := strings.Split(inteiro, ".")
	var b strings.Builder
	for i, g := range grupos {
		switch {
		case g == "":
			return "", ErrFormato
		case i == 0 && len(grupos) > 1 && len(g) > 3:
			return "", ErrFormato
		case i > 0 && len(g) != 3:
			return "", ErrFormato
		}
		b.WriteString(g)
	}
	return b.String(), nil
}

// acumular transforma a sequencia de digitos em Centavos detectando estouro
// antes que ele aconteca. Multiplicacao que estoura int64 nao entra em silencio.
func acumular(digitos string) (Centavos, error) {
	var total Centavos
	for i := 0; i < len(digitos); i++ {
		d := digitos[i]
		if d < '0' || d > '9' {
			return 0, ErrFormato
		}
		unidade := Centavos(d - '0')
		if total > (math.MaxInt64-unidade)/10 {
			return 0, ErrEstouro
		}
		total = total*10 + unidade
	}
	return total, nil
}

// String devolve a escrita brasileira do valor: Centavos(-123456) -> "-R$ 1.234,56".
// Assinatura exigida por fmt.Stringer, o que faz %v e %s formatarem sozinhos.
func (c Centavos) String() string {
	// Negar math.MinInt64 estoura; converter para uint64 pelo complemento nao.
	magnitude := uint64(c)
	sinal := ""
	if c < 0 {
		sinal = "-"
		magnitude = uint64(-(c + 1)) + 1
	}

	digitos := strconv.FormatUint(magnitude, 10)
	if len(digitos) < 3 {
		digitos = strings.Repeat("0", 3-len(digitos)) + digitos
	}
	corte := len(digitos) - 2

	return sinal + "R$ " + agruparMilhar(digitos[:corte]) + "," + digitos[corte:]
}

// agruparMilhar insere o ponto a cada tres digitos, da direita para a esquerda.
func agruparMilhar(inteiro string) string {
	primeiro := len(inteiro) % 3
	if primeiro == 0 {
		primeiro = 3
	}

	var b strings.Builder
	b.WriteString(inteiro[:primeiro])
	for i := primeiro; i < len(inteiro); i += 3 {
		b.WriteByte('.')
		b.WriteString(inteiro[i : i+3])
	}
	return b.String()
}

// Somar devolve a soma de c com os valores recebidos.
// Nao verifica estouro: int64 em centavos cobre ate 92 quatrilhoes de reais,
// e todo valor que entra no sistema passou por Analisar, que ja barra o estouro.
func (c Centavos) Somar(outros ...Centavos) Centavos {
	total := c
	for _, o := range outros {
		total += o
	}
	return total
}
