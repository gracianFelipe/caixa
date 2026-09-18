package aplicacao

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrCredenciaisInvalidas = errors.New("acesso: usuario ou senha invalidos")
	ErrAcessoNaoConfigurado = errors.New("acesso: usuario e hash de senha nao configurados")
)

// DuracaoDaSessao e deslizante: cada uso valido empurra a expiracao.
const DuracaoDaSessao = 30 * 24 * time.Hour

// Acesso e o login do unico usuario. Nao existe tabela de usuarios: o par
// (usuario, hash PHC) vem do ambiente — uma coluna a mais seria coluna morta.
type Acesso struct {
	sessoes RepositorioDeSessoes
	senhas  VerificadorDeSenha
	relogio Relogio
	usuario string
	hash    string
}

func NovoAcesso(sessoes RepositorioDeSessoes, senhas VerificadorDeSenha, relogio Relogio, usuario, hashPHC string) *Acesso {
	return &Acesso{sessoes: sessoes, senhas: senhas, relogio: relogio, usuario: usuario, hash: hashPHC}
}

// Configurado informa se o login esta habilitado. Sem par no ambiente o
// login falha fechado; o acesso por token de maquina continua funcionando.
func (a *Acesso) Configurado() bool {
	return a.usuario != "" && a.hash != ""
}

// Entrar valida as credenciais e abre uma sessao, devolvendo o token que vai
// para o cookie. O hash e SEMPRE verificado, mesmo com usuario errado: quem
// mede o tempo da resposta nao descobre se o usuario existe.
func (a *Acesso) Entrar(ctx context.Context, usuario, senha string) (string, error) {
	if !a.Configurado() {
		return "", ErrAcessoNaoConfigurado
	}

	usuarioBate := subtle.ConstantTimeCompare([]byte(usuario), []byte(a.usuario)) == 1
	senhaBate, err := a.senhas.Confere(a.hash, senha)
	if err != nil {
		return "", fmt.Errorf("verificando senha: %w", err)
	}
	if !usuarioBate || !senhaBate {
		return "", ErrCredenciaisInvalidas
	}

	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		return "", fmt.Errorf("gerando token de sessao: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(bruto)

	if err := a.sessoes.Criar(ctx, hashDoToken(token), a.relogio.Agora().Add(DuracaoDaSessao)); err != nil {
		return "", fmt.Errorf("criando sessao: %w", err)
	}
	return token, nil
}

// Validar diz se o token corresponde a uma sessao viva e a renova.
func (a *Acesso) Validar(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	agora := a.relogio.Agora()
	return a.sessoes.Renovar(ctx, hashDoToken(token), agora, agora.Add(DuracaoDaSessao))
}

// Sair encerra a sessao no servidor: apagar o cookie no cliente nao basta.
func (a *Acesso) Sair(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return a.sessoes.Apagar(ctx, hashDoToken(token))
}

// Usuario e o nome exibido depois do login.
func (a *Acesso) Usuario() string {
	return a.usuario
}

// hashDoToken e o que vai para o banco. sha256 basta: o token tem 256 bits de
// entropia real, nao e senha humana — nao precisa de KDF lento.
func hashDoToken(token string) string {
	soma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(soma[:])
}
