// Package competencia representa o mes contabil a que um lancamento pertence.
// O mes e definido no fuso do usuario, nao em UTC: um PIX as 22:30 de 30/09
// em Sao Paulo e 01:30 UTC de 01/10 e continua sendo de setembro.
package competencia

import (
	"errors"
	"fmt"
	"time"
)

// Competencia e um ano-mes. Comparavel com == por ser struct de dois inteiros.
type Competencia struct {
	ano int
	mes time.Month
}

var ErrFormato = errors.New("competencia: formato invalido, use AAAA-MM")

// Nova monta a competencia a partir de ano e mes validados.
func Nova(ano int, mes time.Month) (Competencia, error) {
	if ano < 1 || mes < time.January || mes > time.December {
		return Competencia{}, ErrFormato
	}
	return Competencia{ano: ano, mes: mes}, nil
}

// Do calcula a competencia de um instante no fuso informado.
// O fuso entra como parametro: o dominio nao carrega tzdata nem decide fuso.
func Do(instante time.Time, fuso *time.Location) Competencia {
	local := instante.In(fuso)
	return Competencia{ano: local.Year(), mes: local.Month()}
}

// Analisar le o formato "AAAA-MM", o mesmo usado na URL e no JSON. Digitos
// verificados a mao: Sscanf aceitaria sinal e espaco ("+026-09") e quebraria
// a ida e volta com String.
func Analisar(texto string) (Competencia, error) {
	if len(texto) != 7 || texto[4] != '-' {
		return Competencia{}, ErrFormato
	}
	for i, r := range texto {
		if i == 4 {
			continue
		}
		if r < '0' || r > '9' {
			return Competencia{}, ErrFormato
		}
	}
	ano := int(texto[0]-'0')*1000 + int(texto[1]-'0')*100 + int(texto[2]-'0')*10 + int(texto[3]-'0')
	mes := int(texto[5]-'0')*10 + int(texto[6]-'0')
	return Nova(ano, time.Month(mes))
}

// Ano e Mes expoem os campos sem permitir que sejam alterados por fora.
func (c Competencia) Ano() int        { return c.ano }
func (c Competencia) Mes() time.Month { return c.mes }

// String devolve "AAAA-MM".
func (c Competencia) String() string {
	return fmt.Sprintf("%04d-%02d", c.ano, c.mes)
}

// PrimeiroDia e a data que vai para a coluna DATE do banco.
func (c Competencia) PrimeiroDia() time.Time {
	return time.Date(c.ano, c.mes, 1, 0, 0, 0, 0, time.UTC)
}

// EhZero informa se a competencia nunca foi definida.
func (c Competencia) EhZero() bool {
	return c == Competencia{}
}
