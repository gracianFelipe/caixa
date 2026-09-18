package aplicacao

import (
	"context"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
	"github.com/gracianFelipe/caixa/internal/dominio/pergunta"
)

// As interfaces moram aqui, no consumidor, e nao em quem implementa.
// O adaptador postgres satisfaz RepositorioDeLancamentos sem importar este
// pacote para "declarar" isso: em Go a satisfacao e implicita e estrutural.
// Cada interface tem so o que este pacote usa; quando um caso de uso novo
// precisar de mais, o metodo entra aqui, e nao no adaptador.

// RepositorioDeLancamentos persiste e consulta o agregado. Salvar recebe o
// evento junto porque os dois entram NA MESMA transacao — e o contrato do
// outbox: nao existe lancamento sem evento nem evento sem lancamento.
type RepositorioDeLancamentos interface {
	Salvar(ctx context.Context, l lancamento.Lancamento, e evento.Evento) error
	DaCompetencia(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error)
	PorID(ctx context.Context, id identidade.ID) (lancamento.Lancamento, bool, error)
	AtribuirCategoria(ctx context.Context, id identidade.ID, cat categoria.ID, origem lancamento.OrigemDaCategoria) error
}

// RepositorioDeOcorrencias grava evidencia, fato e evento na mesma transacao.
// Devolve false quando a ocorrencia ja existia (impressao ou id externo
// repetidos) — a idempotencia mora na constraint, nao em logica de consulta.
type RepositorioDeOcorrencias interface {
	CriarComLancamento(ctx context.Context, o ocorrencia.Ocorrencia, l lancamento.Lancamento, e evento.Evento) (criada bool, err error)
}

// RepositorioDeEventos e o lado consumidor do outbox. ConsumirPendentes abre
// uma transacao, tranca ate `limite` eventos com SKIP LOCKED, chama processar
// para cada um e marca processados no commit. Se processar devolver erro, a
// transacao inteira volta e os eventos continuam pendentes.
type RepositorioDeEventos interface {
	ConsumirPendentes(ctx context.Context, limite int, processar func(evento.Evento) error) (int, error)
}

// RepositorioDePerguntas controla as perguntas abertas no Telegram.
type RepositorioDePerguntas interface {
	Criar(ctx context.Context, p pergunta.Pergunta) error
	AbertaDoLancamento(ctx context.Context, lancamentoID identidade.ID) (pergunta.Pergunta, bool, error)
	MarcarRespondida(ctx context.Context, lancamentoID identidade.ID) error
}

// MensageiroDeCategoria e o que a fila precisa do Telegram — declarado aqui,
// no consumidor; saida/telegram implementa.
type MensageiroDeCategoria interface {
	PerguntarCategoria(ctx context.Context, chatID int64, l lancamento.Lancamento, opcoes []categoria.Categoria) (mensagemID int64, err error)
}

// RepositorioDeRegras entrega as regras ativas e registra o aprendizado que
// vem da resposta humana no Telegram. A ordenacao e problema do dominio.
type RepositorioDeRegras interface {
	Ativas(ctx context.Context) ([]categorizacao.Regra, error)
	RegistrarAprendida(ctx context.Context, cat categoria.ID, padraoExato string) error
}

// RepositorioDeCategorias lista o vocabulario fixo de categorias.
type RepositorioDeCategorias interface {
	Listar(ctx context.Context) ([]categoria.Categoria, error)
}

// Relogio abstrai time.Now para que o caso de uso seja testavel com tempo fixo.
type Relogio interface {
	Agora() time.Time
}
