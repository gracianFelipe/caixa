package aplicacao

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/pergunta"
)

var (
	ErrLancamentoNaoEncontrado = errors.New("fila: lancamento nao encontrado")
	ErrCategoriaDesconhecida   = errors.New("fila: categoria desconhecida")
)

// Fila e o caso de uso do worker: consumir o outbox e conversar com o dono.
type Fila struct {
	eventos     RepositorioDeEventos
	lancamentos RepositorioDeLancamentos
	perguntas   RepositorioDePerguntas
	categorias  RepositorioDeCategorias
	regras      RepositorioDeRegras
	mensageiro  MensageiroDeCategoria
	relogio     Relogio
	chatID      int64
}

func NovaFila(
	eventos RepositorioDeEventos,
	lancamentos RepositorioDeLancamentos,
	perguntas RepositorioDePerguntas,
	categorias RepositorioDeCategorias,
	regras RepositorioDeRegras,
	mensageiro MensageiroDeCategoria,
	relogio Relogio,
	chatID int64,
) *Fila {
	return &Fila{
		eventos: eventos, lancamentos: lancamentos, perguntas: perguntas,
		categorias: categorias, regras: regras, mensageiro: mensageiro,
		relogio: relogio, chatID: chatID,
	}
}

// ProcessarLote consome ate `limite` eventos pendentes. Lancamento criado sem
// categoria vira pergunta no Telegram; todo o resto do evento e consumo puro.
// A pergunta e enviada ANTES de gravada: se cair entre as duas, o evento
// continua pendente e o pior caso e uma pergunta repetida no chat — melhor
// que pergunta registrada que nunca chegou ao dono.
func (f *Fila) ProcessarLote(ctx context.Context, limite int) (int, error) {
	return f.eventos.ConsumirPendentes(ctx, limite, func(e evento.Evento) error {
		if e.Tipo != evento.LancamentoCriado {
			return nil // tipo futuro: consumir sem efeito e melhor que travar a fila
		}

		l, existe, err := f.lancamentos.PorID(ctx, e.LancamentoID)
		if err != nil {
			return err
		}
		if !existe || l.CategoriaID > 0 {
			return nil
		}

		if _, aberta, err := f.perguntas.AbertaDoLancamento(ctx, l.ID); err != nil {
			return err
		} else if aberta {
			return nil
		}

		opcoes, err := f.categorias.Listar(ctx)
		if err != nil {
			return err
		}

		mensagemID, err := f.mensageiro.PerguntarCategoria(ctx, f.chatID, l, opcoes)
		if err != nil {
			return fmt.Errorf("perguntando categoria do lancamento %s: %w", l.ID, err)
		}

		idPergunta, err := identidade.NovaV7(f.relogio.Agora(), rand.Reader)
		if err != nil {
			return err
		}
		p, err := pergunta.Nova(idPergunta, l.ID, f.chatID, f.relogio.Agora())
		if err != nil {
			return err
		}
		p.MensagemID = mensagemID
		return f.perguntas.Criar(ctx, p)
	})
}

// ResponderCategoria aplica a escolha humana: categoria manual no lancamento,
// regra aprendida (exata sobre a contraparte normalizada) e pergunta fechada.
// Devolve o lancamento e a categoria para o chamador editar a mensagem.
func (f *Fila) ResponderCategoria(ctx context.Context, lancamentoID identidade.ID, cat categoria.ID) (lancamento.Lancamento, categoria.Categoria, error) {
	l, existe, err := f.lancamentos.PorID(ctx, lancamentoID)
	if err != nil {
		return lancamento.Lancamento{}, categoria.Categoria{}, err
	}
	if !existe {
		return lancamento.Lancamento{}, categoria.Categoria{}, ErrLancamentoNaoEncontrado
	}

	escolhida, err := f.categoriaPorID(ctx, cat)
	if err != nil {
		return lancamento.Lancamento{}, categoria.Categoria{}, err
	}

	if err := f.lancamentos.AtribuirCategoria(ctx, l.ID, cat, lancamento.CategoriaManual); err != nil {
		return lancamento.Lancamento{}, categoria.Categoria{}, fmt.Errorf("atribuindo categoria: %w", err)
	}

	// Contraparte so de digitos normaliza para vazio; nao ha o que aprender.
	if l.ContraparteNorm != "" {
		if err := f.regras.RegistrarAprendida(ctx, cat, l.ContraparteNorm); err != nil {
			return lancamento.Lancamento{}, categoria.Categoria{}, fmt.Errorf("aprendendo regra: %w", err)
		}
	}

	if err := f.perguntas.MarcarRespondida(ctx, l.ID); err != nil {
		return lancamento.Lancamento{}, categoria.Categoria{}, fmt.Errorf("fechando pergunta: %w", err)
	}

	l.CategoriaID = cat
	l.CategoriaOrigem = lancamento.CategoriaManual
	return l, escolhida, nil
}

func (f *Fila) categoriaPorID(ctx context.Context, id categoria.ID) (categoria.Categoria, error) {
	todas, err := f.categorias.Listar(ctx)
	if err != nil {
		return categoria.Categoria{}, err
	}
	for _, c := range todas {
		if c.ID == id {
			return c, nil
		}
	}
	return categoria.Categoria{}, ErrCategoriaDesconhecida
}
