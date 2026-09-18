// Package orcamento define limites de gasto por categoria e o calculo dos
// niveis de alerta. So aritmetica inteira: gasto e limite sao Centavos.
package orcamento

import (
	"errors"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
)

var (
	ErrCategoriaVazia = errors.New("orcamento: categoria vazia")
	ErrLimiteInvalido = errors.New("orcamento: limite deve ser positivo")
)

// Orcamento e o limite de uma categoria. Competencia zero = padrao do mes:
// vale para qualquer mes que nao tenha limite proprio.
type Orcamento struct {
	Categoria   categoria.ID
	Competencia competencia.Competencia
	Limite      dinheiro.Centavos
}

func Novo(cat categoria.ID, comp competencia.Competencia, limite dinheiro.Centavos) (Orcamento, error) {
	if cat <= 0 {
		return Orcamento{}, ErrCategoriaVazia
	}
	if limite <= 0 {
		return Orcamento{}, ErrLimiteInvalido
	}
	return Orcamento{Categoria: cat, Competencia: comp, Limite: limite}, nil
}

// Limiares de alerta, em percentual do limite.
var Limiares = []int{80, 100}

// Nivel devolve o maior limiar cruzado pelo gasto (0, 80 ou 100). O gasto
// entra como magnitude positiva de saidas. Inteiro puro: gasto*100 nunca
// estoura porque Analisar barra valores perto de MaxInt64/100.
func Nivel(gasto, limite dinheiro.Centavos) int {
	if limite <= 0 || gasto <= 0 {
		return 0
	}
	nivel := 0
	for _, limiar := range Limiares {
		if gasto*100 >= limite*dinheiro.Centavos(limiar) {
			nivel = limiar
		}
	}
	return nivel
}
