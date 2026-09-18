package aplicacao

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
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
	Criados    int
	Duplicados int
	Ignorados  int
}

// Importacao e o caso de uso de carregar evidencias externas no sistema.
type Importacao struct {
	ocorrencias RepositorioDeOcorrencias
	relogio     Relogio
	fuso        *time.Location
}

func NovoServicoDeImportacao(o RepositorioDeOcorrencias, relogio Relogio, fuso *time.Location) *Importacao {
	return &Importacao{ocorrencias: o, relogio: relogio, fuso: fuso}
}

// Importar grava cada item como ocorrencia + lancamento. Item de valor zero e
// linha informativa de extrato (saldo, aviso): ignorado, nao erro. Duplicata
// e detectada pelo banco (UNIQUE em impressao e em id externo), nao por
// logica aqui — importar o mesmo arquivo duas vezes e operacao segura.
func (s *Importacao) Importar(ctx context.Context, origem ocorrencia.Origem, itens []ItemDeExtrato) (ResumoDaImportacao, error) {
	var resumo ResumoDaImportacao

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

		criada, err := s.ocorrencias.CriarComLancamento(ctx, o, l)
		if err != nil {
			return resumo, fmt.Errorf("item %d: %w", i+1, err)
		}
		if criada {
			resumo.Criados++
		} else {
			resumo.Duplicados++
		}
	}
	return resumo, nil
}
