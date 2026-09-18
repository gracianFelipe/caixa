package aplicacao

import (
	"context"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
)

// RepositorioDeSessoes guarda sessoes pelo HASH do token, nunca pelo token.
type RepositorioDeSessoes interface {
	Criar(ctx context.Context, idHash string, expiraEm time.Time) error
	// Renovar confirma que a sessao existe e nao expirou em `agora`, e
	// empurra a expiracao para `novaExpiracao` (sessao deslizante).
	Renovar(ctx context.Context, idHash string, agora, novaExpiracao time.Time) (bool, error)
	Apagar(ctx context.Context, idHash string) error
}

// VerificadorDeSenha compara a senha digitada com o hash PHC guardado.
// O algoritmo (argon2id) e detalhe do adaptador; a aplicacao so pergunta.
type VerificadorDeSenha interface {
	Confere(hashPHC, senha string) (bool, error)
}

// Notificacoes entrega, ate o contexto acabar, cada aviso do banco no canal.
type Notificacoes interface {
	Escutar(ctx context.Context, canal string, receber func(carga string)) error
}

// VisaoDeOrcamento e uma linha da tela de orcamentos: categoria, limite
// vigente (zero = sem limite), se e especifico do mes, e o gasto confirmado.
type VisaoDeOrcamento struct {
	Categoria  categoria.Categoria
	Limite     dinheiro.Centavos
	Especifico bool
	Gasto      dinheiro.Centavos
}

// LimiteVigenteDetalhado e o que o repositorio devolve por categoria quando a
// tela precisa saber se o limite e do mes ou o padrao.
type LimiteVigenteDetalhado struct {
	Categoria  categoria.ID
	Limite     dinheiro.Centavos
	Especifico bool
}

// ConsultaDeOrcamentos completa RepositorioDeOrcamentos para a tela: e uma
// interface separada porque so o servico de orcamentos precisa dela.
type ConsultaDeOrcamentos interface {
	RepositorioDeOrcamentos
	VigentesDetalhados(ctx context.Context, comp competencia.Competencia) ([]LimiteVigenteDetalhado, error)
}
