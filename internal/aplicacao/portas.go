package aplicacao

import (
	"context"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/conciliacao"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
	"github.com/gracianFelipe/caixa/internal/dominio/orcamento"
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
	// CandidatosParaConciliacao devolve lancamentos de valor identico a ate 3
	// dias, nao descartados e SEM evidencia da origem informada — o filtro
	// que impede dois gastos legitimos iguais de virarem um so.
	CandidatosParaConciliacao(ctx context.Context, valor dinheiro.Centavos, instante time.Time, origem ocorrencia.Origem) ([]conciliacao.Candidato, error)
	// GastoConfirmado soma as saidas confirmadas da categoria no mes, como
	// magnitude positiva — e o numero que se compara com o limite.
	GastoConfirmado(ctx context.Context, cat categoria.ID, comp competencia.Competencia) (dinheiro.Centavos, error)
	// FundirProvisorio move as evidencias do provisorio para o destino e
	// marca o provisorio como descartado, numa transacao.
	FundirProvisorio(ctx context.Context, provisorioID, destinoID identidade.ID) error
	ConfirmarProvisorio(ctx context.Context, id identidade.ID) error
	// DaJanela devolve os lancamentos CONFIRMADOS de inicio..fim (inclusivo),
	// em ordem cronologica — a materia-prima do relatorio.
	DaJanela(ctx context.Context, inicio, fim competencia.Competencia) ([]lancamento.Lancamento, error)
}

// RepositorioDeOcorrencias grava evidencia, fato e evento na mesma transacao.
// Devolve false quando a ocorrencia ja existia (impressao ou id externo
// repetidos) — a idempotencia mora na constraint, nao em logica de consulta.
type RepositorioDeOcorrencias interface {
	CriarComLancamento(ctx context.Context, o ocorrencia.Ocorrencia, l lancamento.Lancamento, e evento.Evento) (criada bool, err error)
	// AnexarEvidencia grava a ocorrencia apontando para um lancamento que JA
	// existe (resultado 'conciliou'), sem criar fato novo. false = duplicata.
	AnexarEvidencia(ctx context.Context, o ocorrencia.Ocorrencia, lancamentoID identidade.ID) (anexada bool, err error)
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
	PorID(ctx context.Context, id identidade.ID) (pergunta.Pergunta, bool, error)
	AbertaDoLancamento(ctx context.Context, lancamentoID identidade.ID) (pergunta.Pergunta, bool, error)
	MarcarRespondida(ctx context.Context, lancamentoID identidade.ID) error
}

// Mensageiro e o que a fila precisa do Telegram — declarado aqui, no
// consumidor; saida/telegram implementa. O id da pergunta viaja no botao,
// por isso entra como parametro em vez de nascer depois.
type Mensageiro interface {
	PerguntarCategoria(ctx context.Context, chatID int64, perguntaID identidade.ID, l lancamento.Lancamento, opcoes []categoria.Categoria) (mensagemID int64, err error)
	PerguntarConciliacao(ctx context.Context, chatID int64, perguntaID identidade.ID, provisorio, candidato lancamento.Lancamento) (mensagemID int64, err error)
	EnviarAviso(ctx context.Context, chatID int64, texto string) error
}

// RepositorioDeOrcamentos guarda limites e resolve o vigente (especifico do
// mes vence o padrao).
type RepositorioDeOrcamentos interface {
	Definir(ctx context.Context, o orcamento.Orcamento) error
	LimiteVigente(ctx context.Context, cat categoria.ID, comp competencia.Competencia) (dinheiro.Centavos, bool, error)
	// Vigentes resolve o limite de TODAS as categorias para o mes, com a
	// mesma precedencia (especifico vence padrao).
	Vigentes(ctx context.Context, comp competencia.Competencia) (map[categoria.ID]dinheiro.Centavos, error)
}

// RepositorioDeAlertas registra emissoes. RegistrarSeNovo devolve false se o
// alerta (tipo, chave) ja foi emitido — idempotencia por constraint.
type RepositorioDeAlertas interface {
	RegistrarSeNovo(ctx context.Context, tipo, chave string) (bool, error)
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
