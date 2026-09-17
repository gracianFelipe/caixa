// Package identidade gera e le identificadores UUID versao 7 (RFC 9562).
// Implementado sobre a stdlib porque a camada de aplicacao, que gera os IDs,
// nao importa bibliotecas externas.
package identidade

import (
	"encoding/hex"
	"errors"
	"io"
	"time"
)

// ID e um UUID de 16 bytes. Array, nao slice: comparavel com == e usavel
// como chave de map, e o pgx grava direto na coluna UUID.
type ID [16]byte

var (
	ErrFormato = errors.New("identidade: formato invalido, use 8-4-4-4-12 hexadecimal")
	ErrVazio   = errors.New("identidade: id vazio")
)

// NovaV7 gera um UUIDv7: 48 bits de milissegundos Unix, versao, 74 bits
// aleatorios. O tempo entra como parametro e a fonte de aleatoriedade
// tambem, entao a funcao e testavel sem relogio e sem sorte.
func NovaV7(agora time.Time, aleatorio io.Reader) (ID, error) {
	var id ID
	if _, err := io.ReadFull(aleatorio, id[6:]); err != nil {
		return ID{}, err
	}

	ms := uint64(agora.UnixMilli())
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)

	id[6] = (id[6] & 0x0f) | 0x70 // versao 7 nos 4 bits altos
	id[8] = (id[8] & 0x3f) | 0x80 // variante RFC nos 2 bits altos

	return id, nil
}

// Analisar le a forma canonica "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
func Analisar(texto string) (ID, error) {
	if len(texto) != 36 {
		return ID{}, ErrFormato
	}
	for _, pos := range [...]int{8, 13, 18, 23} {
		if texto[pos] != '-' {
			return ID{}, ErrFormato
		}
	}

	limpo := texto[0:8] + texto[9:13] + texto[14:18] + texto[19:23] + texto[24:36]
	var id ID
	if _, err := hex.Decode(id[:], []byte(limpo)); err != nil {
		return ID{}, ErrFormato
	}
	if id.EhZero() {
		return ID{}, ErrVazio
	}
	return id, nil
}

// String devolve a forma canonica em minusculas.
func (id ID) String() string {
	var buf [36]byte
	hex.Encode(buf[0:8], id[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], id[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], id[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], id[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], id[10:16])
	return string(buf[:])
}

// EhZero informa se o id nunca foi atribuido.
func (id ID) EhZero() bool {
	return id == ID{}
}
