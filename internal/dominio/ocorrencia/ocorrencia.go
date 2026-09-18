// Package ocorrencia modela a evidencia de um movimento: cada mensagem,
// linha de extrato ou e-mail que chega vira uma Ocorrencia. O Lancamento e o
// fato; conciliar e anexar uma segunda evidencia ao mesmo fato, nunca apagar.
package ocorrencia

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

// Origem identifica de onde a evidencia veio. Os valores espelham a tabela
// origens (semente na migracao 002); o banco tem a FK, o dominio tem o tipo.
type Origem int16

const (
	OrigemManual     Origem = 1
	OrigemExtratoOFX Origem = 2
	OrigemEmailBanco Origem = 3
	OrigemTelegram   Origem = 4
)

// EhValida informa se a origem existe na lista fechada.
func (o Origem) EhValida() bool {
	switch o {
	case OrigemManual, OrigemExtratoOFX, OrigemEmailBanco, OrigemTelegram:
		return true
	}
	return false
}

// Resultado e o que aconteceu com a evidencia depois de processada.
type Resultado string

const (
	ResultadoPendente  Resultado = "pendente"
	ResultadoCriou     Resultado = "criou"
	ResultadoConciliou Resultado = "conciliou"
	ResultadoDuplicada Resultado = "duplicada"
	ResultadoErro      Resultado = "erro"
	ResultadoIgnorada  Resultado = "ignorada"
)

var (
	ErrIDVazio        = errors.New("ocorrencia: id vazio")
	ErrOrigemInvalida = errors.New("ocorrencia: origem desconhecida")
	ErrPayloadVazio   = errors.New("ocorrencia: payload vazio")
)

// Ocorrencia nasce pendente e sem lancamento; quem a processa preenche os dois.
type Ocorrencia struct {
	ID           identidade.ID
	Origem       Origem
	IDExterno    string // FITID do OFX, Message-Id do e-mail; vazio = sem id externo
	Impressao    string // sha256 hex do payload normalizado
	Payload      string // carga bruta como texto; numero nunca interpretado aqui
	LancamentoID identidade.ID
	Resultado    Resultado
}

// Nova valida e monta a evidencia, calculando a impressao digital.
func Nova(id identidade.ID, origem Origem, idExterno, payload string) (Ocorrencia, error) {
	if id.EhZero() {
		return Ocorrencia{}, ErrIDVazio
	}
	if !origem.EhValida() {
		return Ocorrencia{}, ErrOrigemInvalida
	}
	normalizado := normalizarPayload(payload)
	if normalizado == "" {
		return Ocorrencia{}, ErrPayloadVazio
	}

	return Ocorrencia{
		ID:        id,
		Origem:    origem,
		IDExterno: strings.TrimSpace(idExterno),
		Impressao: Impressao(payload),
		Payload:   normalizado,
		Resultado: ResultadoPendente,
	}, nil
}

// Impressao e a identidade de conteudo da evidencia: sha256 do payload com
// espacos colapsados. Duas chegadas do mesmo conteudo — mesmo que o banco
// mude espacamento ou quebra de linha — colidem na UNIQUE (origem, impressao).
func Impressao(payload string) string {
	soma := sha256.Sum256([]byte(normalizarPayload(payload)))
	return hex.EncodeToString(soma[:])
}

func normalizarPayload(payload string) string {
	return strings.Join(strings.Fields(payload), " ")
}
