package aplicacao

import (
	"context"
	"fmt"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/relatorio"
)

// RelatorioPronto e o relatorio do dominio mais o que a borda precisa para
// apresentar: nomes das categorias e o texto ja formatado para o Telegram.
type RelatorioPronto struct {
	Relatorio relatorio.Relatorio
	Nomes     map[categoria.ID]string
	Texto     string
}

// Relatorios monta a janela (7 competencias de lancamentos confirmados +
// limites vigentes) e delega o calculo ao dominio.
type Relatorios struct {
	lancamentos RepositorioDeLancamentos
	orcamentos  RepositorioDeOrcamentos
	categorias  RepositorioDeCategorias
}

func NovoServicoDeRelatorios(l RepositorioDeLancamentos, o RepositorioDeOrcamentos, c RepositorioDeCategorias) *Relatorios {
	return &Relatorios{lancamentos: l, orcamentos: o, categorias: c}
}

func (s *Relatorios) Gerar(ctx context.Context, alvo competencia.Competencia) (RelatorioPronto, error) {
	inicio := relatorio.InicioDaJanela(alvo)
	lancamentos, err := s.lancamentos.DaJanela(ctx, inicio, alvo)
	if err != nil {
		return RelatorioPronto{}, fmt.Errorf("carregando janela %s..%s: %w", inicio, alvo, err)
	}
	limites, err := s.orcamentos.Vigentes(ctx, alvo)
	if err != nil {
		return RelatorioPronto{}, fmt.Errorf("carregando limites de %s: %w", alvo, err)
	}
	categorias, err := s.categorias.Listar(ctx)
	if err != nil {
		return RelatorioPronto{}, fmt.Errorf("carregando categorias: %w", err)
	}

	nomes := make(map[categoria.ID]string, len(categorias))
	for _, c := range categorias {
		nomes[c.ID] = c.Nome
	}

	r := relatorio.Gerar(relatorio.Janela{Alvo: alvo, Lancamentos: lancamentos, Limites: limites})
	return RelatorioPronto{Relatorio: r, Nomes: nomes, Texto: relatorio.Formatar(r, nomes)}, nil
}

// HoraDoRelatorioMensal e quando o relatorio do mes fechado sai: dia 1 as
// 08:00 no fuso do dono. "Dia 30" nao existe em fevereiro — dia 1 sobre o
// mes anterior e a correcao do plano.
const HoraDoRelatorioMensal = 8

// CompetenciaAgendada devolve o mes fechado a reportar se `agoraLocal` (ja no
// fuso do dono) passou do horario do dia 1; senao, false. Pura: o agendador
// do worker so a chama a cada tique e deixa a idempotencia para RegistrarSeNovo.
func CompetenciaAgendada(agoraLocal time.Time) (competencia.Competencia, bool) {
	if agoraLocal.Day() != 1 || agoraLocal.Hour() < HoraDoRelatorioMensal {
		// Fora do dia 1, ainda vale mandar o do mes anterior se o worker
		// esteve fora no dia 1: a chave em alertas impede repeticao, entao
		// "atrasado" e seguro. So nao mandamos ANTES das 08:00 do dia 1.
		if agoraLocal.Day() == 1 {
			return competencia.Competencia{}, false
		}
	}
	anterior := agoraLocal.AddDate(0, 0, -agoraLocal.Day()) // ultimo dia do mes passado
	c, err := competencia.Nova(anterior.Year(), anterior.Month())
	if err != nil {
		return competencia.Competencia{}, false
	}
	return c, true
}
