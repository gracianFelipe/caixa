// Package relogio e a implementacao real de aplicacao.Relogio. E o unico
// lugar fora de cmd e testes onde time.Now aparece.
package relogio

import "time"

// Sistema le o relogio da maquina. Struct vazia: nao tem estado, e o tipo
// existe so para satisfazer a interface.
type Sistema struct{}

func (Sistema) Agora() time.Time {
	return time.Now()
}
