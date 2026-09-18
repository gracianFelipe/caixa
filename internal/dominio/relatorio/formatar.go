package relatorio

import (
	"fmt"
	"strings"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
)

// Formatar produz o texto do Telegram. Recebe os nomes das categorias por
// parametro: o dominio conhece ids, quem resolve nomes e a aplicacao.
func Formatar(r Relatorio, nomes map[categoria.ID]string) string {
	nome := func(id categoria.ID) string {
		if n, ok := nomes[id]; ok {
			return n
		}
		if id == 0 {
			return "sem categoria"
		}
		return fmt.Sprintf("categoria %d", id)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Relatorio %s\n", r.Competencia)
	fmt.Fprintf(&b, "saidas: %s", r.TotalSaidas)
	if r.SaidasMesAnterior > 0 {
		fmt.Fprintf(&b, " (%s no mes anterior)", r.SaidasMesAnterior)
	}
	fmt.Fprintf(&b, "\nentradas: %s\nsaldo: %s\n", r.TotalEntradas, r.Saldo)

	if len(r.PorCategoria) > 0 {
		b.WriteString("\npor categoria:\n")
		for i, c := range r.PorCategoria {
			if i == 5 {
				b.WriteString("  ...\n")
				break
			}
			fmt.Fprintf(&b, "  %s: %s (%d)\n", nome(c.Categoria), c.Total, c.Quantidade)
		}
	}

	if len(r.Sinais) == 0 {
		b.WriteString("\nnenhum sinal de alerta.\n")
		return b.String()
	}

	b.WriteString("\nsinais:\n")
	for _, s := range r.Sinais {
		switch s.Tipo {
		case SinalRepeticao:
			fmt.Fprintf(&b, "  • %s: %d gastos pequenos somam %s\n", nome(s.Categoria), s.Severidade, s.Valor)
		case SinalAssinatura:
			fmt.Fprintf(&b, "  • %s cobra %s ha %d meses — ainda usa?\n", s.Contraparte, s.Valor, s.Severidade)
		case SinalEscalada:
			fmt.Fprintf(&b, "  • %s subiu %d%% em meses seguidos (agora %s)\n", nome(s.Categoria), s.Severidade, s.Valor)
		case SinalEstouro:
			fmt.Fprintf(&b, "  • %s em %d%% do limite (%s)\n", nome(s.Categoria), s.Severidade, s.Valor)
		case SinalAtipico:
			fmt.Fprintf(&b, "  • %s fora do padrao: %s\n", s.Contraparte, s.Valor)
		default:
			fmt.Fprintf(&b, "  • %s: %s\n", s.Tipo, s.Detalhe)
		}
	}
	return b.String()
}
