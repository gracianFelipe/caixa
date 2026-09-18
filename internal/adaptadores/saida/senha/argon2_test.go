package senha

import (
	"errors"
	"strings"
	"testing"
)

func TestGerarEConferir(t *testing.T) {
	var a Argon2id
	phc, err := a.Gerar("correta horse battery staple")
	if err != nil {
		t.Fatalf("Gerar: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Errorf("formato PHC inesperado: %s", phc)
	}

	ok, err := a.Confere(phc, "correta horse battery staple")
	if err != nil || !ok {
		t.Fatalf("senha certa: (%v, %v)", ok, err)
	}
	ok, err = a.Confere(phc, "errada")
	if err != nil || ok {
		t.Errorf("senha errada: (%v, %v)", ok, err)
	}

	// Dois hashes da mesma senha diferem (sal aleatorio) e ambos conferem.
	phc2, _ := a.Gerar("correta horse battery staple")
	if phc2 == phc {
		t.Error("sal repetido")
	}
	if ok, _ := a.Confere(phc2, "correta horse battery staple"); !ok {
		t.Error("segundo hash nao confere")
	}
}

func TestConfereParametrosDoHash(t *testing.T) {
	// Hash com custos menores que os atuais continua valido: os parametros
	// vem do proprio PHC.
	var a Argon2id
	phc, _ := a.Gerar("x")
	barato := strings.Replace(phc, "m=65536,t=3,p=2", "m=65536,t=3,p=2", 1) // mesmo hash; garante parse
	if ok, err := a.Confere(barato, "x"); err != nil || !ok {
		t.Errorf("(%v, %v)", ok, err)
	}
}

func TestConfereFormatoInvalido(t *testing.T) {
	var a Argon2id
	casos := []string{
		"",
		"senha-em-texto-puro",
		"$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=0,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=2$@@@$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA",
	}
	for _, phc := range casos {
		t.Run(phc, func(t *testing.T) {
			if _, err := a.Confere(phc, "x"); !errors.Is(err, ErrFormatoPHC) {
				t.Errorf("Confere(%q) = %v, queria ErrFormatoPHC", phc, err)
			}
		})
	}
}
