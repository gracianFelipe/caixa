// Package aplicacao orquestra os casos de uso. Nao contem regra de negocio
// (isso e do dominio) nem detalhe de transporte ou banco (isso e dos
// adaptadores): recebe dados ja tipados, coordena, devolve.
package aplicacao

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// Lancamentos e o servico de aplicacao do agregado. Ponteiro como receptor
// porque a struct carrega dependencias compartilhadas e nao deve ser copiada.
type Lancamentos struct {
	repo    RepositorioDeLancamentos
	regras  RepositorioDeRegras
	relogio Relogio
	fuso    *time.Location
}

// NovoServicoDeLancamentos recebe as dependencias por parametro: quem monta
// e o cmd, que conhece o banco e o fuso; este pacote so conhece as interfaces.
func NovoServicoDeLancamentos(repo RepositorioDeLancamentos, regras RepositorioDeRegras, relogio Relogio, fuso *time.Location) *Lancamentos {
	return &Lancamentos{repo: repo, regras: regras, relogio: relogio, fuso: fuso}
}

// Registrar cria, classifica e persiste um lancamento. Erros do dominio sobem
// intactos (o handler decide o status HTTP com errors.Is); erros de
// infraestrutura sobem embrulhados com contexto do que estava sendo feito.
func (s *Lancamentos) Registrar(ctx context.Context, d lancamento.Dados) (lancamento.Lancamento, error) {
	id, err := identidade.NovaV7(s.relogio.Agora(), rand.Reader)
	if err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("gerando id: %w", err)
	}

	l, err := lancamento.Novo(id, d, s.fuso)
	if err != nil {
		return lancamento.Lancamento{}, err
	}

	l, err = s.classificar(ctx, l)
	if err != nil {
		return lancamento.Lancamento{}, err
	}

	e, err := s.eventoDeCriacao(l)
	if err != nil {
		return lancamento.Lancamento{}, err
	}

	if err := s.repo.Salvar(ctx, l, e); err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("salvando lancamento %s: %w", l.ID, err)
	}
	return l, nil
}

// Capturar e o caso de uso do Atalho do iOS: valor em texto brasileiro,
// instante = agora, meio padrao pix. Valor sem sinal explicito e saida —
// captura rapida existe para registrar gasto no momento em que acontece.
func (s *Lancamentos) Capturar(ctx context.Context, valorTexto, contraparte, meioTexto string) (lancamento.Lancamento, error) {
	valor, err := dinheiro.Analisar(valorTexto)
	if err != nil {
		return lancamento.Lancamento{}, err
	}
	semSinal := !strings.HasPrefix(strings.TrimSpace(valorTexto), "+") &&
		!strings.HasPrefix(strings.TrimSpace(valorTexto), "-")
	if semSinal && valor > 0 {
		valor = -valor
	}

	if strings.TrimSpace(meioTexto) == "" {
		meioTexto = string(lancamento.MeioPix)
	}

	return s.Registrar(ctx, lancamento.Dados{
		OcorridoEm:  s.relogio.Agora(),
		Valor:       valor,
		Meio:        lancamento.Meio(meioTexto),
		Contraparte: contraparte,
	})
}

func (s *Lancamentos) eventoDeCriacao(l lancamento.Lancamento) (evento.Evento, error) {
	id, err := identidade.NovaV7(s.relogio.Agora(), rand.Reader)
	if err != nil {
		return evento.Evento{}, fmt.Errorf("gerando id do evento: %w", err)
	}
	e, err := evento.Novo(id, evento.LancamentoCriado, l.ID, s.relogio.Agora())
	if err != nil {
		return evento.Evento{}, err
	}
	return e, nil
}

// classificar aplica as regras ativas; sem correspondencia, o lancamento
// segue pendente — e a fila de pergunta da Fase 3, nao um erro. Falha ao
// CARREGAR regras e erro de verdade: engolir deixaria tudo pendente em
// silencio e a fila cresceria sem ninguem perceber o motivo.
func (s *Lancamentos) classificar(ctx context.Context, l lancamento.Lancamento) (lancamento.Lancamento, error) {
	regras, err := s.regras.Ativas(ctx)
	if err != nil {
		return lancamento.Lancamento{}, fmt.Errorf("carregando regras: %w", err)
	}

	resultado, ok := categorizacao.Classificar(l.ContraparteNorm, regras)
	if !ok {
		return l, nil
	}
	return l.ComCategoria(resultado.Categoria, lancamento.CategoriaPorRegra)
}

// Listar devolve os lancamentos de uma competencia.
func (s *Lancamentos) Listar(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error) {
	ls, err := s.repo.DaCompetencia(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("listando competencia %s: %w", c, err)
	}
	return ls, nil
}
