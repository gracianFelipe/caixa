package aplicacao

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/conciliacao"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
	"github.com/gracianFelipe/caixa/internal/dominio/orcamento"
	"github.com/gracianFelipe/caixa/internal/dominio/pergunta"
)

var (
	ErrLancamentoNaoEncontrado = errors.New("fila: lancamento nao encontrado")
	ErrCategoriaDesconhecida   = errors.New("fila: categoria desconhecida")
	ErrPerguntaNaoEncontrada   = errors.New("fila: pergunta nao encontrada")
)

// Fila e o caso de uso do worker: consumir o outbox e conversar com o dono.
type Fila struct {
	eventos     RepositorioDeEventos
	lancamentos RepositorioDeLancamentos
	perguntas   RepositorioDePerguntas
	categorias  RepositorioDeCategorias
	regras      RepositorioDeRegras
	orcamentos  RepositorioDeOrcamentos
	alertas     RepositorioDeAlertas
	mensageiro  Mensageiro
	relogio     Relogio
	chatID      int64
}

// DependenciasDaFila agrupa as oito dependencias: struct nomeada em vez de
// oito parametros posicionais que qualquer troca silenciosa quebraria.
type DependenciasDaFila struct {
	Eventos     RepositorioDeEventos
	Lancamentos RepositorioDeLancamentos
	Perguntas   RepositorioDePerguntas
	Categorias  RepositorioDeCategorias
	Regras      RepositorioDeRegras
	Orcamentos  RepositorioDeOrcamentos
	Alertas     RepositorioDeAlertas
	Mensageiro  Mensageiro
	Relogio     Relogio
	ChatID      int64
}

func NovaFila(d DependenciasDaFila) *Fila {
	return &Fila{
		eventos: d.Eventos, lancamentos: d.Lancamentos, perguntas: d.Perguntas,
		categorias: d.Categorias, regras: d.Regras, orcamentos: d.Orcamentos,
		alertas: d.Alertas, mensageiro: d.Mensageiro, relogio: d.Relogio,
		chatID: d.ChatID,
	}
}

// ProcessarLote consome ate `limite` eventos pendentes. Cada lancamento_criado
// segue um de tres caminhos: provisorio → pergunta de conciliacao; confirmado
// sem categoria → pergunta de categoria; confirmado categorizado → checagem
// de orcamento. Perguntas sao enviadas ANTES de gravadas: se cair no meio, o
// evento fica pendente e o pior caso e mensagem repetida — nunca pergunta
// fantasma que o dono jamais recebeu.
func (f *Fila) ProcessarLote(ctx context.Context, limite int) (int, error) {
	return f.eventos.ConsumirPendentes(ctx, limite, func(e evento.Evento) error {
		if e.Tipo != evento.LancamentoCriado {
			return nil
		}

		l, existe, err := f.lancamentos.PorID(ctx, e.LancamentoID)
		if err != nil {
			return err
		}
		if !existe || l.Situacao == lancamento.SituacaoDescartada {
			return nil
		}

		if _, aberta, err := f.perguntas.AbertaDoLancamento(ctx, l.ID); err != nil {
			return err
		} else if aberta {
			return nil
		}

		if l.Situacao == lancamento.SituacaoProvisoria {
			return f.perguntarConciliacao(ctx, l)
		}
		if l.CategoriaID == 0 {
			return f.perguntarCategoria(ctx, l)
		}
		return f.verificarOrcamento(ctx, l)
	})
}

func (f *Fila) perguntarCategoria(ctx context.Context, l lancamento.Lancamento) error {
	opcoes, err := f.categorias.Listar(ctx)
	if err != nil {
		return err
	}

	idPergunta, err := identidade.NovaV7(f.relogio.Agora(), rand.Reader)
	if err != nil {
		return err
	}
	p, err := pergunta.Nova(idPergunta, l.ID, f.chatID, f.relogio.Agora())
	if err != nil {
		return err
	}

	mensagemID, err := f.mensageiro.PerguntarCategoria(ctx, f.chatID, p.ID, l, opcoes)
	if err != nil {
		return fmt.Errorf("perguntando categoria do lancamento %s: %w", l.ID, err)
	}
	p.MensagemID = mensagemID
	return f.perguntas.Criar(ctx, p)
}

// perguntarConciliacao recalcula os candidatos do provisorio — a conciliacao
// e deterministica, entao recalcular da o mesmo vencedor sem coluna extra.
// Se o cenario mudou (candidato descartado, score caiu), o provisorio e
// confirmado sem incomodar o dono.
func (f *Fila) perguntarConciliacao(ctx context.Context, provisorio lancamento.Lancamento) error {
	origem := origemProvavel(provisorio)
	candidatos, err := f.lancamentos.CandidatosParaConciliacao(ctx, provisorio.Valor, provisorio.OcorridoEm, origem)
	if err != nil {
		return err
	}

	// O proprio provisorio pode voltar como candidato de si mesmo? Nao: ele
	// tem evidencia da propria origem, e o filtro exclui. Mas por robustez:
	elegiveis := candidatos[:0]
	for _, c := range candidatos {
		if c.Lancamento.ID != provisorio.ID {
			elegiveis = append(elegiveis, c)
		}
	}

	vencedor, pontos := conciliacao.Melhor(conciliacao.Evidencia{
		Valor:           provisorio.Valor,
		OcorridoEm:      provisorio.OcorridoEm,
		ContraparteNorm: provisorio.ContraparteNorm,
		Meio:            provisorio.Meio,
		Origem:          origem,
	}, elegiveis)

	if conciliacao.Decidir(pontos) == conciliacao.LancamentoNovo {
		if err := f.lancamentos.ConfirmarProvisorio(ctx, provisorio.ID); err != nil {
			return err
		}
		if provisorio.CategoriaID == 0 {
			return f.perguntarCategoria(ctx, provisorio)
		}
		return f.verificarOrcamento(ctx, provisorio)
	}

	idPergunta, err := identidade.NovaV7(f.relogio.Agora(), rand.Reader)
	if err != nil {
		return err
	}
	p, err := pergunta.NovaDeConciliacao(idPergunta, provisorio.ID, vencedor.Lancamento.ID, f.chatID, f.relogio.Agora())
	if err != nil {
		return err
	}

	mensagemID, err := f.mensageiro.PerguntarConciliacao(ctx, f.chatID, p.ID, provisorio, vencedor.Lancamento)
	if err != nil {
		return fmt.Errorf("perguntando conciliacao do lancamento %s: %w", provisorio.ID, err)
	}
	p.MensagemID = mensagemID
	return f.perguntas.Criar(ctx, p)
}

// origemProvavel infere a origem da evidencia do provisorio. Hoje so a
// importacao de extrato cria provisorios; quando o e-mail (Fase 7) tambem
// criar, a origem vira dado da pergunta.
func origemProvavel(lancamento.Lancamento) ocorrencia.Origem {
	return ocorrencia.OrigemExtratoOFX
}

// ResponderCategoria aplica a escolha humana: categoria manual no lancamento,
// regra aprendida (exata sobre a contraparte normalizada) e pergunta fechada.
// Depois, checa o orcamento da categoria recem-atribuida.
func (f *Fila) ResponderCategoria(ctx context.Context, perguntaID identidade.ID, cat categoria.ID) (lancamento.Lancamento, categoria.Categoria, error) {
	p, existe, err := f.perguntas.PorID(ctx, perguntaID)
	if err != nil {
		return lancamento.Lancamento{}, categoria.Categoria{}, err
	}
	if !existe {
		return lancamento.Lancamento{}, categoria.Categoria{}, ErrPerguntaNaoEncontrada
	}

	l, existe, err := f.lancamentos.PorID(ctx, p.LancamentoID)
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
	if err := f.verificarOrcamento(ctx, l); err != nil {
		return lancamento.Lancamento{}, categoria.Categoria{}, err
	}
	return l, escolhida, nil
}

// ResponderConciliacao resolve o provisorio: mesmo gasto funde (evidencias
// migram, provisorio descartado); gasto novo confirma — e, se ainda sem
// categoria, a pergunta de categoria sai na hora.
func (f *Fila) ResponderConciliacao(ctx context.Context, perguntaID identidade.ID, mesmoGasto bool) (pergunta.Pergunta, error) {
	p, existe, err := f.perguntas.PorID(ctx, perguntaID)
	if err != nil {
		return pergunta.Pergunta{}, err
	}
	if !existe || p.Tipo != pergunta.TipoConciliacao {
		return pergunta.Pergunta{}, ErrPerguntaNaoEncontrada
	}

	if mesmoGasto {
		if err := f.lancamentos.FundirProvisorio(ctx, p.LancamentoID, p.Referencia); err != nil {
			return pergunta.Pergunta{}, fmt.Errorf("fundindo provisorio: %w", err)
		}
		if err := f.perguntas.MarcarRespondida(ctx, p.LancamentoID); err != nil {
			return pergunta.Pergunta{}, err
		}
		return p, nil
	}

	if err := f.lancamentos.ConfirmarProvisorio(ctx, p.LancamentoID); err != nil {
		return pergunta.Pergunta{}, fmt.Errorf("confirmando provisorio: %w", err)
	}
	if err := f.perguntas.MarcarRespondida(ctx, p.LancamentoID); err != nil {
		return pergunta.Pergunta{}, err
	}

	l, existe, err := f.lancamentos.PorID(ctx, p.LancamentoID)
	if err != nil || !existe {
		return p, err
	}
	if l.CategoriaID == 0 {
		return p, f.perguntarCategoria(ctx, l)
	}
	return p, f.verificarOrcamento(ctx, l)
}

// verificarOrcamento compara o gasto confirmado do mes com o limite vigente
// e avisa UMA vez por limiar cruzado — o "uma vez" e a constraint de alertas,
// nao memoria de processo.
func (f *Fila) verificarOrcamento(ctx context.Context, l lancamento.Lancamento) error {
	if l.CategoriaID == 0 || !l.EhSaida() {
		return nil
	}

	limite, existe, err := f.orcamentos.LimiteVigente(ctx, l.CategoriaID, l.Competencia)
	if err != nil {
		return err
	}
	if !existe {
		return nil
	}

	gasto, err := f.lancamentos.GastoConfirmado(ctx, l.CategoriaID, l.Competencia)
	if err != nil {
		return err
	}

	nivel := orcamento.Nivel(gasto, limite)
	if nivel == 0 {
		return nil
	}

	nomeDaCategoria := fmt.Sprintf("categoria %d", l.CategoriaID)
	if c, err := f.categoriaPorID(ctx, l.CategoriaID); err == nil {
		nomeDaCategoria = c.Nome
	}

	for _, limiar := range orcamento.Limiares {
		if nivel < limiar {
			continue
		}
		chave := fmt.Sprintf("%d:%s:%d", l.CategoriaID, l.Competencia, limiar)
		novo, err := f.alertas.RegistrarSeNovo(ctx, "orcamento", chave)
		if err != nil {
			return fmt.Errorf("registrando alerta: %w", err)
		}
		if !novo {
			continue
		}
		texto := fmt.Sprintf("orcamento de %s em %s: %s de %s (%d%%)",
			nomeDaCategoria, l.Competencia, gasto, limite, limiar)
		if err := f.mensageiro.EnviarAviso(ctx, f.chatID, texto); err != nil {
			return fmt.Errorf("avisando orcamento: %w", err)
		}
	}
	return nil
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
