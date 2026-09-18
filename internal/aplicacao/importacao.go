package aplicacao

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
	"github.com/gracianFelipe/caixa/internal/dominio/conciliacao"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

// ItemDeExtrato e o DTO de entrada da importacao. Quem le OFX e o adaptador;
// quem converte Transacao->ItemDeExtrato e o cmd. Este pacote nao conhece
// nenhum formato de arquivo — so o que precisa para criar fato e evidencia.
type ItemDeExtrato struct {
	OcorridoEm  time.Time
	Valor       dinheiro.Centavos
	Meio        lancamento.Meio
	Contraparte string
	IDExterno   string
	Payload     string
}

// ResumoDaImportacao conta o destino de cada item. Contagens, nunca conteudo:
// e o que pode ir para log e stdout sem vazar PII.
type ResumoDaImportacao struct {
	Criados     int
	Conciliados int // evidencia anexada a lancamento que ja existia
	Provisorios int // faixa 60-84: criado marcado para revisao humana
	Duplicados  int
	Ignorados   int
}

// Importacao e o caso de uso de carregar evidencias externas no sistema.
type Importacao struct {
	ocorrencias RepositorioDeOcorrencias
	lancamentos RepositorioDeLancamentos
	regras      RepositorioDeRegras
	relogio     Relogio
	fuso        *time.Location
}

func NovoServicoDeImportacao(o RepositorioDeOcorrencias, l RepositorioDeLancamentos, regras RepositorioDeRegras, relogio Relogio, fuso *time.Location) *Importacao {
	return &Importacao{ocorrencias: o, lancamentos: l, regras: regras, relogio: relogio, fuso: fuso}
}

// Importar grava cada item como ocorrencia + lancamento. Item de valor zero e
// linha informativa de extrato (saldo, aviso): ignorado, nao erro. Duplicata
// e detectada pelo banco (UNIQUE em impressao e em id externo), nao por
// logica aqui — importar o mesmo arquivo duas vezes e operacao segura.
func (s *Importacao) Importar(ctx context.Context, origem ocorrencia.Origem, itens []ItemDeExtrato) (ResumoDaImportacao, error) {
	var resumo ResumoDaImportacao

	// Uma carga de regras por lote, nao por item: o lote e atomico no tempo.
	regras, err := s.regras.Ativas(ctx)
	if err != nil {
		return resumo, fmt.Errorf("carregando regras: %w", err)
	}

	for i, item := range itens {
		if item.Valor == 0 {
			resumo.Ignorados++
			continue
		}

		idOcorrencia, err := identidade.NovaV7(s.relogio.Agora(), rand.Reader)
		if err != nil {
			return resumo, fmt.Errorf("gerando id da ocorrencia: %w", err)
		}
		o, err := ocorrencia.Nova(idOcorrencia, origem, item.IDExterno, item.Payload)
		if err != nil {
			return resumo, fmt.Errorf("item %d: %w", i+1, err)
		}

		idLancamento, err := identidade.NovaV7(s.relogio.Agora(), rand.Reader)
		if err != nil {
			return resumo, fmt.Errorf("gerando id do lancamento: %w", err)
		}
		l, err := lancamento.Novo(idLancamento, lancamento.Dados{
			OcorridoEm:  item.OcorridoEm,
			Valor:       item.Valor,
			Meio:        item.Meio,
			Contraparte: item.Contraparte,
		}, s.fuso)
		if err != nil {
			return resumo, fmt.Errorf("item %d: %w", i+1, err)
		}

		// Classificacao por regra tambem na importacao; origem continua
		// "regra" (a origem "importacao" fica para fonte que ja traga a
		// categoria dela, ex. CSV com coluna propria).
		if resultado, ok := categorizacao.Classificar(l.ContraparteNorm, regras); ok {
			if l, err = l.ComCategoria(resultado.Categoria, lancamento.CategoriaPorRegra); err != nil {
				return resumo, fmt.Errorf("item %d: %w", i+1, err)
			}
		}

		// Nivel 2 da deduplicacao: o mesmo gasto vindo por OUTRA fonte anexa
		// evidencia em vez de criar fato. O repositorio ja filtrou quem tem
		// evidencia desta origem (defesa dos dois gastos iguais no mesmo dia).
		candidatos, err := s.lancamentos.CandidatosParaConciliacao(ctx, item.Valor, item.OcorridoEm, origem)
		if err != nil {
			return resumo, fmt.Errorf("item %d: buscando candidatos: %w", i+1, err)
		}
		vencedor, pontos := conciliacao.Melhor(conciliacao.Evidencia{
			Valor:           item.Valor,
			OcorridoEm:      item.OcorridoEm,
			ContraparteNorm: l.ContraparteNorm,
			Meio:            l.Meio,
			Origem:          origem,
		}, candidatos)

		switch conciliacao.Decidir(pontos) {
		case conciliacao.Conciliar:
			anexada, err := s.ocorrencias.AnexarEvidencia(ctx, o, vencedor.Lancamento.ID)
			if err != nil {
				return resumo, fmt.Errorf("item %d: conciliando: %w", i+1, err)
			}
			if anexada {
				resumo.Conciliados++
			} else {
				resumo.Duplicados++
			}
			continue
		case conciliacao.Perguntar:
			// Faixa 60-84: cria o fato marcado para revisao — nada some e
			// nada e fundido sem confirmacao. A pergunta no Telegram e a
			// proxima spec; ate la o provisorio aparece nas listas.
			l = l.Provisorio()
		}

		idEvento, err := identidade.NovaV7(s.relogio.Agora(), rand.Reader)
		if err != nil {
			return resumo, fmt.Errorf("gerando id do evento: %w", err)
		}
		e, err := evento.Novo(idEvento, evento.LancamentoCriado, l.ID, s.relogio.Agora())
		if err != nil {
			return resumo, fmt.Errorf("item %d: %w", i+1, err)
		}

		criada, err := s.ocorrencias.CriarComLancamento(ctx, o, l, e)
		if err != nil {
			return resumo, fmt.Errorf("item %d: %w", i+1, err)
		}
		switch {
		case !criada:
			resumo.Duplicados++
		case l.Situacao == lancamento.SituacaoProvisoria:
			resumo.Provisorios++
		default:
			resumo.Criados++
		}
	}
	return resumo, nil
}
