// Package aplicacao orquestra os casos de uso. Nao contem regra de negocio
// (isso e do dominio) nem detalhe de transporte ou banco (isso e dos
// adaptadores): recebe dados ja tipados, coordena, devolve.
package aplicacao

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// Lancamentos e o servico de aplicacao do agregado. Ponteiro como receptor
// porque a struct carrega dependencias compartilhadas e nao deve ser copiada.
type Lancamentos struct {
	repo    RepositorioDeLancamentos
	relogio Relogio
	fuso    *time.Location
}

// NovoServicoDeLancamentos recebe as dependencias por parametro: quem monta
// e o cmd, que conhece o banco e o fuso; este pacote so conhece as interfaces.
func NovoServicoDeLancamentos(repo RepositorioDeLancamentos, relogio Relogio, fuso *time.Location) *Lancamentos {
	return &Lancamentos{repo: repo, relogio: relogio, fuso: fuso}
}

// Registrar cria e persiste um lancamento. Erros do dominio sobem intactos
// (o handler decide o status HTTP com errors.Is); erros de infraestrutura
// sobem embrulhados com contexto do que estava sendo feito.
func (s *Lancamentos) Registrar(ctx context.Context, d lancamento.Dados) (lancamento.Lancamento, error) {
	id, err := identidade.NovaV7(s.relogio.Agora(), rand.Reader)
	if err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("gerando id: %w", err)
	}

	l, err := lancamento.Novo(id, d, s.fuso)
	if err != nil {
		return lancamento.Lancamento{}, err
	}

	if err := s.repo.Salvar(ctx, l); err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("salvando lancamento %s: %w", l.ID, err)
	}
	return l, nil
}

// Listar devolve os lancamentos de uma competencia.
func (s *Lancamentos) Listar(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error) {
	ls, err := s.repo.DaCompetencia(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("listando competencia %s: %w", c, err)
	}
	return ls, nil
}
