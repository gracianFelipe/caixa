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

// Analisar le o formato "AAAA-MM", o mesmo usado na URL e no JSON.
func Analisar(texto string) (Competencia, error) {
	if len(texto) != 7 || texto[4] != '-' {
		return Competencia{}, ErrFormato
	}
	var ano, mes int
	if _, err := fmt.Sscanf(texto, "%4d-%2d", &ano, &mes); err != nil {
		return Competencia{}, ErrFormato
	}
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
