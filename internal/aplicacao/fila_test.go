package aplicacao

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/evento"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/pergunta"
)

// --- fakes ---

type eventosEmMemoria struct {
	pendentes []evento.Evento
}

func (r *eventosEmMemoria) ConsumirPendentes(_ context.Context, limite int, processar func(evento.Evento) error) (int, error) {
	n := 0
	for len(r.pendentes) > 0 && n < limite {
		e := r.pendentes[0]
		if err := processar(e); err != nil {
			return n, err // como no banco: erro devolve o lote, eventos ficam
		}
		r.pendentes = r.pendentes[1:]
		n++
	}
	return n, nil
}

type lancamentosPorID struct {
	repoEmMemoria
	atribuicoes map[identidade.ID]categoria.ID
}

func (r *lancamentosPorID) PorID(_ context.Context, id identidade.ID) (lancamento.Lancamento, bool, error) {
	for _, l := range r.salvos {
		if l.ID == id {
			return l, true, nil
		}
	}
	return lancamento.Lancamento{}, false, nil
}

func (r *lancamentosPorID) AtribuirCategoria(_ context.Context, id identidade.ID, cat categoria.ID, origem lancamento.OrigemDaCategoria) error {
	if r.atribuicoes == nil {
		r.atribuicoes = map[identidade.ID]categoria.ID{}
	}
	r.atribuicoes[id] = cat
	return nil
}

type perguntasEmMemoria struct {
	criadas     []pergunta.Pergunta
	respondidas []identidade.ID
}

func (r *perguntasEmMemoria) Criar(_ context.Context, p pergunta.Pergunta) error {
	r.criadas = append(r.criadas, p)
	return nil
}

func (r *perguntasEmMemoria) AbertaDoLancamento(_ context.Context, lancamentoID identidade.ID) (pergunta.Pergunta, bool, error) {
	for _, p := range r.criadas {
		if p.LancamentoID == lancamentoID && p.Estado == pergunta.Aberta {
			return p, true, nil
		}
	}
	return pergunta.Pergunta{}, false, nil
}

func (r *perguntasEmMemoria) MarcarRespondida(_ context.Context, lancamentoID identidade.ID) error {
	r.respondidas = append(r.respondidas, lancamentoID)
	return nil
}

type categoriasFixas []categoria.Categoria

func (c categoriasFixas) Listar(context.Context) ([]categoria.Categoria, error) { return c, nil }

type regrasQueAprendem struct {
	regrasFixas
	aprendidas map[string]categoria.ID
}

func (r *regrasQueAprendem) RegistrarAprendida(_ context.Context, cat categoria.ID, padrao string) error {
	if r.aprendidas == nil {
		r.aprendidas = map[string]categoria.ID{}
	}
	r.aprendidas[padrao] = cat
	return nil
}

type mensageiroFalso struct {
	enviadas []string // contrapartes perguntadas
	falha    error
}

func (m *mensageiroFalso) PerguntarCategoria(_ context.Context, chatID int64, l lancamento.Lancamento, _ []categoria.Categoria) (int64, error) {
	if m.falha != nil {
		return 0, m.falha
	}
	m.enviadas = append(m.enviadas, l.Contraparte)
	return int64(1000 + len(m.enviadas)), nil
}

// regrasFixas do lancamentos_test nao tem RegistrarAprendida; embrulha aqui.
func (r regrasFixas) RegistrarAprendida(context.Context, categoria.ID, string) error { return nil }

// --- helpers ---

func filaDeTeste(t *testing.T, lancs *lancamentosPorID, eventos *eventosEmMemoria, perguntas *perguntasEmMemoria, mensageiro *mensageiroFalso) *Fila {
	t.Helper()
	mercado, _ := categoria.Nova(1, "mercado")
	restaurante, _ := categoria.Nova(2, "restaurante")
	return NovaFila(
		eventos, lancs, perguntas,
		categoriasFixas{mercado, restaurante},
		&regrasQueAprendem{},
		mensageiro,
		relogioFixo(time.Now()),
		777,
	)
}

func lancamentoSalvo(t *testing.T, repo *lancamentosPorID, contraparte string, comCategoria bool) lancamento.Lancamento {
	t.Helper()
	id, _ := identidade.NovaV7(time.Now(), leituraFixa{})
	l, err := lancamento.Novo(id, lancamento.Dados{
		OcorridoEm: time.Now(), Valor: -100, Meio: lancamento.MeioPix, Contraparte: contraparte,
	}, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}
	if comCategoria {
		l, _ = l.ComCategoria(1, lancamento.CategoriaPorRegra)
	}
	repo.salvos = append(repo.salvos, l)
	return l
}

// leituraFixa gera bytes deterministicos diferentes por chamada.
type leituraFixa struct{}

var contadorLeitura byte

func (leituraFixa) Read(p []byte) (int, error) {
	contadorLeitura++
	for i := range p {
		p[i] = contadorLeitura
	}
	return len(p), nil
}

func eventoDe(t *testing.T, l lancamento.Lancamento) evento.Evento {
	t.Helper()
	id, _ := identidade.NovaV7(time.Now(), leituraFixa{})
	e, err := evento.Novo(id, evento.LancamentoCriado, l.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// --- testes ---

func TestProcessarLote(t *testing.T) {
	lancs := &lancamentosPorID{}
	pendente := lancamentoSalvo(t, lancs, "LOJA MISTERIOSA", false)
	categorizado := lancamentoSalvo(t, lancs, "IFOOD", true)

	eventos := &eventosEmMemoria{pendentes: []evento.Evento{
		eventoDe(t, pendente),
		eventoDe(t, categorizado),
	}}
	perguntas := &perguntasEmMemoria{}
	mensageiro := &mensageiroFalso{}

	f := filaDeTeste(t, lancs, eventos, perguntas, mensageiro)

	n, err := f.ProcessarLote(context.Background(), 10)
	if err != nil {
		t.Fatalf("ProcessarLote: %v", err)
	}
	if n != 2 {
		t.Errorf("processou %d eventos, queria 2", n)
	}

	// So o pendente vira pergunta; o categorizado e consumo puro.
	if len(mensageiro.enviadas) != 1 || mensageiro.enviadas[0] != "LOJA MISTERIOSA" {
		t.Errorf("perguntas enviadas: %v", mensageiro.enviadas)
	}
	if len(perguntas.criadas) != 1 || perguntas.criadas[0].LancamentoID != pendente.ID {
		t.Fatalf("perguntas criadas: %+v", perguntas.criadas)
	}
	if perguntas.criadas[0].MensagemID == 0 {
		t.Error("pergunta gravada sem o id da mensagem enviada")
	}

	// Reprocessar o mesmo lancamento nao pergunta de novo.
	eventos.pendentes = []evento.Evento{eventoDe(t, pendente)}
	if _, err := f.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(mensageiro.enviadas) != 1 {
		t.Error("pergunta duplicada para o mesmo lancamento")
	}
}

func TestProcessarLoteFalhaNoEnvio(t *testing.T) {
	lancs := &lancamentosPorID{}
	pendente := lancamentoSalvo(t, lancs, "LOJA", false)
	eventos := &eventosEmMemoria{pendentes: []evento.Evento{eventoDe(t, pendente)}}
	perguntas := &perguntasEmMemoria{}
	mensageiro := &mensageiroFalso{falha: errors.New("telegram fora do ar")}

	f := filaDeTeste(t, lancs, eventos, perguntas, mensageiro)

	if _, err := f.ProcessarLote(context.Background(), 10); !errors.Is(err, mensageiro.falha) {
		t.Fatalf("erro = %v", err)
	}
	// O evento continua pendente para a proxima rodada.
	if len(eventos.pendentes) != 1 {
		t.Error("evento foi consumido apesar da falha")
	}
	if len(perguntas.criadas) != 0 {
		t.Error("pergunta nao pode ser criada quando o envio falha")
	}
}

func TestResponderCategoria(t *testing.T) {
	lancs := &lancamentosPorID{}
	l := lancamentoSalvo(t, lancs, "Pão de Açúcar 123", false)
	perguntas := &perguntasEmMemoria{}
	regras := &regrasQueAprendem{}

	mercado, _ := categoria.Nova(1, "mercado")
	restaurante, _ := categoria.Nova(2, "restaurante")
	f := NovaFila(&eventosEmMemoria{}, lancs, perguntas,
		categoriasFixas{mercado, restaurante}, regras, &mensageiroFalso{},
		relogioFixo(time.Now()), 777)

	atualizado, escolhida, err := f.ResponderCategoria(context.Background(), l.ID, 1)
	if err != nil {
		t.Fatalf("ResponderCategoria: %v", err)
	}

	if atualizado.CategoriaID != 1 || atualizado.CategoriaOrigem != lancamento.CategoriaManual {
		t.Errorf("lancamento = (%d, %s)", atualizado.CategoriaID, atualizado.CategoriaOrigem)
	}
	if escolhida.Nome != "mercado" {
		t.Errorf("categoria = %+v", escolhida)
	}
	if lancs.atribuicoes[l.ID] != 1 {
		t.Error("atribuicao nao chegou ao repositorio")
	}
	if regras.aprendidas["PAO DE ACUCAR"] != 1 {
		t.Errorf("regra aprendida = %v, queria PAO DE ACUCAR -> 1", regras.aprendidas)
	}
	if len(perguntas.respondidas) != 1 || perguntas.respondidas[0] != l.ID {
		t.Error("pergunta nao foi fechada")
	}
}

func TestResponderCategoriaErros(t *testing.T) {
	lancs := &lancamentosPorID{}
	l := lancamentoSalvo(t, lancs, "LOJA", false)
	f := filaDeTeste(t, lancs, &eventosEmMemoria{}, &perguntasEmMemoria{}, &mensageiroFalso{})
	ctx := context.Background()

	if _, _, err := f.ResponderCategoria(ctx, identidade.ID{9, 9}, 1); !errors.Is(err, ErrLancamentoNaoEncontrado) {
		t.Errorf("lancamento inexistente: erro = %v", err)
	}
	if _, _, err := f.ResponderCategoria(ctx, l.ID, 99); !errors.Is(err, ErrCategoriaDesconhecida) {
		t.Errorf("categoria inexistente: erro = %v", err)
	}
}

// Compile-time: o fake de lancamentos satisfaz a porta completa.
var _ RepositorioDeLancamentos = (*lancamentosPorID)(nil)

// O repoEmMemoria antigo nao implementa PorID/AtribuirCategoria/evento; os
// testes de lancamentos usam lancamentosPorID por embutimento. A verificacao
// de que Salvar recebe o evento fica no proprio fake:
func TestSalvarRecebeEvento(t *testing.T) {
	repo := &lancamentosPorID{}
	s := NovoServicoDeLancamentos(repo, regrasFixas{}, relogioFixo(time.Now()), saoPaulo)

	l, err := s.Registrar(context.Background(), dadosValidos())
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.eventos) != 1 {
		t.Fatalf("Salvar recebeu %d eventos, queria 1", len(repo.eventos))
	}
	e := repo.eventos[0]
	if e.Tipo != evento.LancamentoCriado || e.LancamentoID != l.ID {
		t.Errorf("evento = %+v", e)
	}
	if strings.Contains(string(e.Tipo), " ") {
		t.Error("tipo de evento com espaco")
	}
}
