package ocorrencia

import (
	"errors"
	"strings"
	"testing"

	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
)

var idFixo = identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 2}

func TestNova(t *testing.T) {
	o, err := Nova(idFixo, OrigemExtratoOFX, "  FIT123  ", "<STMTTRN> <TRNAMT>-47.90 </STMTTRN>")
	if err != nil {
		t.Fatalf("Nova devolveu erro: %v", err)
	}

	if o.Resultado != ResultadoPendente {
		t.Errorf("Resultado = %q, queria pendente", o.Resultado)
	}
	if o.IDExterno != "FIT123" {
		t.Errorf("IDExterno = %q, queria sem espacos", o.IDExterno)
	}
	if !o.LancamentoID.EhZero() {
		t.Error("ocorrencia nova nao pode apontar para lancamento")
	}
	if len(o.Impressao) != 64 {
		t.Errorf("Impressao tem %d caracteres, queria 64 (sha256 hex)", len(o.Impressao))
	}
}

func TestNovaErro(t *testing.T) {
	casos := []struct {
		nome    string
		id      identidade.ID
		origem  Origem
		payload string
		erro    error
	}{
		{"id vazio", identidade.ID{}, OrigemManual, "x", ErrIDVazio},
		{"origem zero", idFixo, 0, "x", ErrOrigemInvalida},
		{"origem fora da lista", idFixo, 99, "x", ErrOrigemInvalida},
		{"payload vazio", idFixo, OrigemManual, "", ErrPayloadVazio},
		{"payload so espaco", idFixo, OrigemManual, " \n\t ", ErrPayloadVazio},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := Nova(c.id, c.origem, "", c.payload); !errors.Is(err, c.erro) {
				t.Errorf("Nova devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

// TestImpressaoNormaliza: a mesma carga com espacamento diferente tem a mesma
// impressao — e o que faz a idempotencia sobreviver a mudancas cosmeticas.
func TestImpressaoNormaliza(t *testing.T) {
	a := Impressao("PIX  SUPERMERCADO\n-47.90")
	b := Impressao(" PIX SUPERMERCADO -47.90 ")
	c := Impressao("PIX SUPERMERCADO -47.91")

	if a != b {
		t.Error("espacamento diferente mudou a impressao")
	}
	if a == c {
		t.Error("conteudo diferente colidiu na impressao")
	}
	if strings.ToLower(a) != a {
		t.Error("impressao deveria ser hex minusculo")
	}
}
