// Package senha implementa a verificacao de senha com argon2id no formato
// PHC ($argon2id$v=19$m=…,t=…,p=…$salt$hash), o mesmo que outras ferramentas
// geram e leem — o hash do ambiente nao fica preso a este codigo.
package senha

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
)

var _ aplicacao.VerificadorDeSenha = Argon2id{}

var ErrFormatoPHC = errors.New("senha: hash fora do formato PHC argon2id")

// Parametros recomendados pela OWASP para argon2id (64 MiB, 3 passadas, 2
// threads): ~100 ms numa maquina comum — lento para ataque, imperceptivel
// para um login por dia.
const (
	memoriaKiB = 64 * 1024
	passadas   = 3
	paralelo   = 2
	tamanhoSal = 16
	tamanhoKey = 32
)

type Argon2id struct{}

// Gerar produz o PHC de uma senha nova (usado pelo caixactl senha).
func (Argon2id) Gerar(senha string) (string, error) {
	sal := make([]byte, tamanhoSal)
	if _, err := rand.Read(sal); err != nil {
		return "", fmt.Errorf("gerando sal: %w", err)
	}
	chave := argon2.IDKey([]byte(senha), sal, passadas, memoriaKiB, paralelo, tamanhoKey)
	codificar := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memoriaKiB, passadas, paralelo, codificar(sal), codificar(chave)), nil
}

// Confere recalcula com os parametros gravados NO hash (nao os constantes
// acima): um hash antigo gerado com outros custos continua valido.
func (Argon2id) Confere(hashPHC, senha string) (bool, error) {
	partes := strings.Split(hashPHC, "$")
	if len(partes) != 6 || partes[0] != "" || partes[1] != "argon2id" {
		return false, ErrFormatoPHC
	}
	var versao int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &versao); err != nil || versao != argon2.Version {
		return false, ErrFormatoPHC
	}
	var m uint32
	var t, p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return false, ErrFormatoPHC
	}
	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil || len(sal) == 0 {
		return false, ErrFormatoPHC
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil || len(esperado) == 0 {
		return false, ErrFormatoPHC
	}

	obtido := argon2.IDKey([]byte(senha), sal, uint32(t), m, p, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(obtido, esperado) == 1, nil
}
