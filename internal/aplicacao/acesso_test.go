package aplicacao

import (
	"context"
	"errors"
	"testing"
	"time"
)

type sessoesEmMemoria struct {
	porHash map[string]time.Time // hash -> expira_em
}

func novasSessoes() *sessoesEmMemoria { return &sessoesEmMemoria{porHash: map[string]time.Time{}} }

func (s *sessoesEmMemoria) Criar(_ context.Context, idHash string, expiraEm time.Time) error {
	s.porHash[idHash] = expiraEm
	return nil
}

func (s *sessoesEmMemoria) Renovar(_ context.Context, idHash string, agora, nova time.Time) (bool, error) {
	exp, ok := s.porHash[idHash]
	if !ok || !exp.After(agora) {
		return false, nil
	}
	s.porHash[idHash] = nova
	return true, nil
}

func (s *sessoesEmMemoria) Apagar(_ context.Context, idHash string) error {
	delete(s.porHash, idHash)
	return nil
}

// senhaFixa aceita exatamente uma senha e conta as verificacoes: o teste de
// "sempre verifica" depende disso.
type senhaFixa struct {
	certa        string
	verificacoes int
}

func (s *senhaFixa) Confere(_ string, senha string) (bool, error) {
	s.verificacoes++
	return senha == s.certa, nil
}

func TestAcessoEntrarEValidar(t *testing.T) {
	sessoes := novasSessoes()
	agora := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	a := NovoAcesso(sessoes, &senhaFixa{certa: "correta"}, relogioFixo(agora), "felipe", "$argon2id$hash")
	ctx := context.Background()

	token, err := a.Entrar(ctx, "felipe", "correta")
	if err != nil {
		t.Fatalf("Entrar: %v", err)
	}
	if len(token) < 40 {
		t.Errorf("token curto demais: %d chars", len(token))
	}
	if _, guardouCru := sessoes.porHash[token]; guardouCru {
		t.Error("o token cru foi para o repositorio; deveria ser o hash")
	}

	ok, err := a.Validar(ctx, token)
	if err != nil || !ok {
		t.Fatalf("Validar = (%v, %v), queria sessao valida", ok, err)
	}
	if ok, _ := a.Validar(ctx, "token-inventado"); ok {
		t.Error("token inventado validou")
	}
	if ok, _ := a.Validar(ctx, ""); ok {
		t.Error("token vazio validou")
	}

	if err := a.Sair(ctx, token); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.Validar(ctx, token); ok {
		t.Error("sessao continua valida depois de Sair")
	}
}

func TestAcessoSessaoExpiraEDesliza(t *testing.T) {
	sessoes := novasSessoes()
	inicio := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	relogio := &relogioMovel{agora: inicio}
	a := NovoAcesso(sessoes, &senhaFixa{certa: "x"}, relogio, "felipe", "h")
	ctx := context.Background()

	token, _ := a.Entrar(ctx, "felipe", "x")

	// 29 dias depois: valida e renova por mais 30.
	relogio.agora = inicio.Add(29 * 24 * time.Hour)
	if ok, _ := a.Validar(ctx, token); !ok {
		t.Fatal("sessao de 29 dias deveria valer")
	}
	// 29 + 29 dias: ainda vale porque deslizou.
	relogio.agora = inicio.Add(58 * 24 * time.Hour)
	if ok, _ := a.Validar(ctx, token); !ok {
		t.Fatal("sessao renovada deveria valer")
	}
	// 31 dias sem uso: expirou.
	relogio.agora = inicio.Add(90 * 24 * time.Hour)
	if ok, _ := a.Validar(ctx, token); ok {
		t.Error("sessao sem uso por 31 dias deveria expirar")
	}
}

func TestAcessoCredenciaisInvalidasSempreVerificaHash(t *testing.T) {
	senhas := &senhaFixa{certa: "correta"}
	a := NovoAcesso(novasSessoes(), senhas, relogioFixo(time.Now()), "felipe", "h")
	ctx := context.Background()

	casos := []struct{ usuario, senha string }{
		{"felipe", "errada"},
		{"outro", "correta"},
		{"outro", "errada"},
		{"", ""},
	}
	for _, c := range casos {
		if _, err := a.Entrar(ctx, c.usuario, c.senha); !errors.Is(err, ErrCredenciaisInvalidas) {
			t.Errorf("Entrar(%q, %q) = %v, queria ErrCredenciaisInvalidas", c.usuario, c.senha, err)
		}
	}
	// Uma verificacao de hash por tentativa, inclusive com usuario errado:
	// o tempo de resposta nao revela se o usuario existe.
	if senhas.verificacoes != len(casos) {
		t.Errorf("hash verificado %d vezes, queria %d", senhas.verificacoes, len(casos))
	}
}

func TestAcessoNaoConfigurado(t *testing.T) {
	a := NovoAcesso(novasSessoes(), &senhaFixa{}, relogioFixo(time.Now()), "", "")
	if a.Configurado() {
		t.Error("sem usuario/hash nao deveria estar configurado")
	}
	if _, err := a.Entrar(context.Background(), "x", "y"); !errors.Is(err, ErrAcessoNaoConfigurado) {
		t.Errorf("erro = %v", err)
	}
}

type relogioMovel struct{ agora time.Time }

func (r *relogioMovel) Agora() time.Time { return r.agora }
