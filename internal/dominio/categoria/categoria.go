// Package categoria e o vocabulario de destino do dinheiro. As categorias
// sao fixas e semeadas por migracao; o dominio so as representa e valida.
package categoria

import (
	"errors"
	"strings"
)

// ID identifica uma categoria. int16 espelha o SMALLINT do banco; zero
// significa "sem categoria" e nunca e um ID valido.
type ID int16

var (
	ErrIDInvalido = errors.New("categoria: id deve ser positivo")
	ErrNomeVazio  = errors.New("categoria: nome vazio")
)

type Categoria struct {
	ID   ID
	Nome string
}

// Nova valida o par vindo do banco ou de teste.
func Nova(id ID, nome string) (Categoria, error) {
	if id <= 0 {
		return Categoria{}, ErrIDInvalido
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return Categoria{}, ErrNomeVazio
	}
	return Categoria{ID: id, Nome: nome}, nil
}
