package aplicacao

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
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

type lancamentosDaFila struct {
	repoEmMemoria
	atribuicoes map[identidade.ID]categoria.ID
	gastos      map[categoria.ID]dinheiro.Centavos
	fundidos    [][2]identidade.ID
	confirmados []identidade.ID
}

func (r *lancamentosDaFila) PorID(_ context.Context, id identidade.ID) (lancamento.Lancamento, bool, error) {
	for _, l := range r.salvos {
		if l.ID == id {
			return l, true, nil
		}
	}
	return lancamento.Lancamento{}, false, nil
}

func (r *lancamentosDaFila) AtribuirCategoria(_ context.Context, id identidade.ID, cat categoria.ID, _ lancamento.OrigemDaCategoria) error {
	if r.atribuicoes == nil {
		r.atribuicoes = map[identidade.ID]categoria.ID{}
	}
	r.atribuicoes[id] = cat
	return nil
}

func (r *lancamentosDaFila) GastoConfirmado(_ context.Context, cat categoria.ID, _ competencia.Competencia) (dinheiro.Centavos, error) {
	return r.gastos[cat], nil
}

func (r *lancamentosDaFila) FundirProvisorio(_ context.Context, provisorio, destino identidade.ID) error {
	r.fundidos = append(r.fundidos, [2]identidade.ID{provisorio, destino})
	return nil
}

func (r *lancamentosDaFila) ConfirmarProvisorio(_ context.Context, id identidade.ID) error {
	r.confirmados = append(r.confirmados, id)
	for i, l := range r.salvos {
		if l.ID == id {
			r.salvos[i].Situacao = lancamento.SituacaoConfirmada
		}
	}
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

func (r *perguntasEmMemoria) PorID(_ context.Context, id identidade.ID) (pergunta.Pergunta, bool, error) {
	for _, p := range r.criadas {
		if p.ID == id {
			return p, true, nil
		}
	}
	return pergunta.Pergunta{}, false, nil
}

func (r *perguntasEmMemoria) AbertaDoLancamento(_ context.Context, lancamentoID identidade.ID) (pergunta.Pergunta, bool, error) {
	for _, p := range r.criadas {
		if p.LancamentoID == lancamentoID && p.Estado == pergunta.Aberta && !contemID(r.respondidas, lancamentoID) {
			return p, true, nil
		}
	}
	return pergunta.Pergunta{}, false, nil
}

func (r *perguntasEmMemoria) MarcarRespondida(_ context.Context, lancamentoID identidade.ID) error {
	r.respondidas = append(r.respondidas, lancamentoID)
	return nil
}

func contemID(ids []identidade.ID, alvo identidade.ID) bool {
	for _, id := range ids {
		if id == alvo {
			return true
		}
	}
	return false
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

func (r regrasFixas) RegistrarAprendida(context.Context, categoria.ID, string) error { return nil }

type orcamentosFixos map[categoria.ID]dinheiro.Centavos

func (o orcamentosFixos) Definir(context.Context, orcamento.Orcamento) error { return nil }
func (o orcamentosFixos) LimiteVigente(_ context.Context, cat categoria.ID, _ competencia.Competencia) (dinheiro.Centavos, bool, error) {
	limite, existe := o[cat]
	return limite, existe, nil
}

type alertasEmMemoria struct {
	emitidos map[string]bool
}

func (a *alertasEmMemoria) RegistrarSeNovo(_ context.Context, tipo, chave string) (bool, error) {
	if a.emitidos == nil {
		a.emitidos = map[string]bool{}
	}
	completa := tipo + "|" + chave
	if a.emitidos[completa] {
		return false, nil
	}
	a.emitidos[completa] = true
	return true, nil
}

type mensageiroFalso struct {
	categorias   []string // contrapartes das perguntas de categoria
	conciliacoes []string
	avisos       []string
	falha        error
}

func (m *mensageiroFalso) PerguntarCategoria(_ context.Context, _ int64, _ identidade.ID, l lancamento.Lancamento, _ []categoria.Categoria) (int64, error) {
	if m.falha != nil {
		return 0, m.falha
	}
	m.categorias = append(m.categorias, l.Contraparte)
	return int64(1000 + len(m.categorias)), nil
}

func (m *mensageiroFalso) PerguntarConciliacao(_ context.Context, _ int64, _ identidade.ID, provisorio, _ lancamento.Lancamento) (int64, error) {
	if m.falha != nil {
		return 0, m.falha
	}
	m.conciliacoes = append(m.conciliacoes, provisorio.Contraparte)
	return int64(2000 + len(m.conciliacoes)), nil
}

func (m *mensageiroFalso) EnviarAviso(_ context.Context, _ int64, texto string) error {
	m.avisos = append(m.avisos, texto)
	return nil
}

// --- helpers ---

type ambiente struct {
	lancs      *lancamentosDaFila
	eventos    *eventosEmMemoria
	perguntas  *perguntasEmMemoria
	regras     *regrasQueAprendem
	orcamentos orcamentosFixos
	alertas    *alertasEmMemoria
	mensageiro *mensageiroFalso
	fila       *Fila
}

func novoAmbiente(t *testing.T) *ambiente {
	t.Helper()
	mercado, _ := categoria.Nova(1, "mercado")
	restaurante, _ := categoria.Nova(2, "restaurante")

	a := &ambiente{
		lancs:      &lancamentosDaFila{},
		eventos:    &eventosEmMemoria{},
		perguntas:  &perguntasEmMemoria{},
		regras:     &regrasQueAprendem{},
		orcamentos: orcamentosFixos{},
		alertas:    &alertasEmMemoria{},
		mensageiro: &mensageiroFalso{},
	}
	a.fila = NovaFila(DependenciasDaFila{
		Eventos: a.eventos, Lancamentos: a.lancs, Perguntas: a.perguntas,
		Categorias: categoriasFixas{mercado, restaurante}, Regras: a.regras,
		Orcamentos: a.orcamentos, Alertas: a.alertas, Mensageiro: a.mensageiro,
		Relogio: relogioFixo(time.Now()), ChatID: 777,
	})
	return a
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

func lancamentoSalvo(t *testing.T, a *ambiente, contraparte string, ajustes ...func(*lancamento.Lancamento)) lancamento.Lancamento {
	t.Helper()
	id, _ := identidade.NovaV7(time.Now(), leituraFixa{})
	l, err := lancamento.Novo(id, lancamento.Dados{
		OcorridoEm: time.Date(2026, time.September, 10, 15, 0, 0, 0, time.UTC),
		Valor:      -4790, Meio: lancamento.MeioPix, Contraparte: contraparte,
	}, saoPaulo)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ajustes {
		f(&l)
	}
	a.lancs.salvos = append(a.lancs.salvos, l)
	return l
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

func TestProcessarLotePerguntaCategoria(t *testing.T) {
	a := novoAmbiente(t)
	pendente := lancamentoSalvo(t, a, "LOJA MISTERIOSA")
	categorizado := lancamentoSalvo(t, a, "IFOOD", func(l *lancamento.Lancamento) {
		*l, _ = l.ComCategoria(2, lancamento.CategoriaPorRegra)
	})
	a.eventos.pendentes = []evento.Evento{eventoDe(t, pendente), eventoDe(t, categorizado)}

	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if len(a.mensageiro.categorias) != 1 || a.mensageiro.categorias[0] != "LOJA MISTERIOSA" {
		t.Errorf("perguntas de categoria: %v", a.mensageiro.categorias)
	}
	if len(a.perguntas.criadas) != 1 || a.perguntas.criadas[0].MensagemID == 0 {
		t.Fatalf("perguntas criadas: %+v", a.perguntas.criadas)
	}

	// Reprocessar nao pergunta de novo.
	a.eventos.pendentes = []evento.Evento{eventoDe(t, pendente)}
	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(a.mensageiro.categorias) != 1 {
		t.Error("pergunta duplicada")
	}
}

func TestProcessarLoteFalhaNoEnvio(t *testing.T) {
	a := novoAmbiente(t)
	pendente := lancamentoSalvo(t, a, "LOJA")
	a.eventos.pendentes = []evento.Evento{eventoDe(t, pendente)}
	a.mensageiro.falha = errors.New("telegram fora do ar")

	if _, err := a.fila.ProcessarLote(context.Background(), 10); !errors.Is(err, a.mensageiro.falha) {
		t.Fatalf("erro = %v", err)
	}
	if len(a.eventos.pendentes) != 1 {
		t.Error("evento consumido apesar da falha")
	}
	if len(a.perguntas.criadas) != 0 {
		t.Error("pergunta criada apesar da falha no envio")
	}
}

// origensDe monta a lista de origens de um candidato (2 = extrato_ofx: a
// mesma do provisorio, o que poe a pontuacao na faixa 60-84: 50+25+10-20).
func origensDe(ids ...int16) []ocorrencia.Origem {
	out := make([]ocorrencia.Origem, 0, len(ids))
	for _, id := range ids {
		out = append(out, ocorrencia.Origem(id))
	}
	return out
}

func TestProvisorioViraPergunta(t *testing.T) {
	a := novoAmbiente(t)

	existente := lancamentoSalvo(t, a, "OUTRA LOJA")
	provisorio := lancamentoSalvo(t, a, "LOJA DUVIDOSA", func(l *lancamento.Lancamento) {
		*l = l.Provisorio()
	})
	a.lancs.candidatos = []conciliacao.Candidato{
		{Lancamento: existente, Origens: origensDe(2)},
	}
	a.eventos.pendentes = []evento.Evento{eventoDe(t, provisorio)}

	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if len(a.mensageiro.conciliacoes) != 1 {
		t.Fatalf("conciliacoes perguntadas: %v", a.mensageiro.conciliacoes)
	}
	p := a.perguntas.criadas[0]
	if p.Tipo != pergunta.TipoConciliacao || p.Referencia != existente.ID {
		t.Errorf("pergunta = %+v", p)
	}
}

func TestProvisorioSemCandidatoConfirma(t *testing.T) {
	a := novoAmbiente(t)
	provisorio := lancamentoSalvo(t, a, "LOJA SOZINHA", func(l *lancamento.Lancamento) {
		*l = l.Provisorio()
	})
	a.eventos.pendentes = []evento.Evento{eventoDe(t, provisorio)}

	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if len(a.lancs.confirmados) != 1 || a.lancs.confirmados[0] != provisorio.ID {
		t.Error("provisorio sem candidato deveria ser confirmado")
	}
	// Confirmado e pendente de categoria: pergunta de categoria na sequencia.
	if len(a.mensageiro.categorias) != 1 {
		t.Errorf("perguntas de categoria: %v", a.mensageiro.categorias)
	}
}

func TestResponderCategoria(t *testing.T) {
	a := novoAmbiente(t)
	l := lancamentoSalvo(t, a, "Pão de Açúcar 123")
	a.eventos.pendentes = []evento.Evento{eventoDe(t, l)}
	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	perguntaID := a.perguntas.criadas[0].ID

	atualizado, escolhida, err := a.fila.ResponderCategoria(context.Background(), perguntaID, 1)
	if err != nil {
		t.Fatalf("ResponderCategoria: %v", err)
	}

	if atualizado.CategoriaID != 1 || atualizado.CategoriaOrigem != lancamento.CategoriaManual {
		t.Errorf("lancamento = (%d, %s)", atualizado.CategoriaID, atualizado.CategoriaOrigem)
	}
	if escolhida.Nome != "mercado" {
		t.Errorf("categoria = %+v", escolhida)
	}
	if a.lancs.atribuicoes[l.ID] != 1 {
		t.Error("atribuicao nao chegou ao repositorio")
	}
	if a.regras.aprendidas["PAO DE ACUCAR"] != 1 {
		t.Errorf("regra aprendida = %v", a.regras.aprendidas)
	}
	if !contemID(a.perguntas.respondidas, l.ID) {
		t.Error("pergunta nao foi fechada")
	}
}

func TestResponderCategoriaErros(t *testing.T) {
	a := novoAmbiente(t)
	l := lancamentoSalvo(t, a, "LOJA")
	a.eventos.pendentes = []evento.Evento{eventoDe(t, l)}
	_, _ = a.fila.ProcessarLote(context.Background(), 10)
	perguntaID := a.perguntas.criadas[0].ID
	ctx := context.Background()

	if _, _, err := a.fila.ResponderCategoria(ctx, identidade.ID{9, 9}, 1); !errors.Is(err, ErrPerguntaNaoEncontrada) {
		t.Errorf("pergunta inexistente: %v", err)
	}
	if _, _, err := a.fila.ResponderCategoria(ctx, perguntaID, 99); !errors.Is(err, ErrCategoriaDesconhecida) {
		t.Errorf("categoria inexistente: %v", err)
	}
}

func TestResponderConciliacao(t *testing.T) {
	preparar := func(t *testing.T) (*ambiente, identidade.ID, identidade.ID, identidade.ID) {
		a := novoAmbiente(t)
		existente := lancamentoSalvo(t, a, "OUTRA LOJA")
		provisorio := lancamentoSalvo(t, a, "LOJA DUVIDOSA", func(l *lancamento.Lancamento) {
			*l = l.Provisorio()
		})
		a.lancs.candidatos = []conciliacao.Candidato{
			{Lancamento: existente, Origens: origensDe(2)},
		}
		a.eventos.pendentes = []evento.Evento{eventoDe(t, provisorio)}
		if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		return a, a.perguntas.criadas[0].ID, provisorio.ID, existente.ID
	}

	t.Run("mesmo gasto funde", func(t *testing.T) {
		a, perguntaID, provisorioID, existenteID := preparar(t)

		if _, err := a.fila.ResponderConciliacao(context.Background(), perguntaID, true); err != nil {
			t.Fatal(err)
		}
		if len(a.lancs.fundidos) != 1 || a.lancs.fundidos[0] != [2]identidade.ID{provisorioID, existenteID} {
			t.Errorf("fusoes: %v", a.lancs.fundidos)
		}
		if len(a.lancs.confirmados) != 0 {
			t.Error("mesmo gasto nao confirma o provisorio")
		}
	})

	t.Run("gasto novo confirma e pergunta categoria", func(t *testing.T) {
		a, perguntaID, provisorioID, _ := preparar(t)

		if _, err := a.fila.ResponderConciliacao(context.Background(), perguntaID, false); err != nil {
			t.Fatal(err)
		}
		if len(a.lancs.confirmados) != 1 || a.lancs.confirmados[0] != provisorioID {
			t.Errorf("confirmados: %v", a.lancs.confirmados)
		}
		if len(a.mensageiro.categorias) != 1 {
			t.Errorf("pergunta de categoria apos confirmar: %v", a.mensageiro.categorias)
		}
	})
}

func TestVerificarOrcamentoAvisaUmaVez(t *testing.T) {
	a := novoAmbiente(t)
	a.orcamentos[1] = 80000                                       // limite R$ 800,00
	a.lancs.gastos = map[categoria.ID]dinheiro.Centavos{1: 65000} // 81%

	l := lancamentoSalvo(t, a, "SUPERMERCADO", func(l *lancamento.Lancamento) {
		*l, _ = l.ComCategoria(1, lancamento.CategoriaPorRegra)
	})
	a.eventos.pendentes = []evento.Evento{eventoDe(t, l)}
	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if len(a.mensageiro.avisos) != 1 || !strings.Contains(a.mensageiro.avisos[0], "80%") {
		t.Fatalf("avisos = %v", a.mensageiro.avisos)
	}

	// Segundo gasto na mesma faixa: alerta ja emitido, nada novo.
	l2 := lancamentoSalvo(t, a, "MERCADINHO", func(l *lancamento.Lancamento) {
		*l, _ = l.ComCategoria(1, lancamento.CategoriaPorRegra)
	})
	a.eventos.pendentes = []evento.Evento{eventoDe(t, l2)}
	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(a.mensageiro.avisos) != 1 {
		t.Errorf("aviso repetido: %v", a.mensageiro.avisos)
	}

	// Cruzou 100%: um aviso novo (e so o de 100, porque o de 80 ja foi).
	a.lancs.gastos[1] = 81000
	l3 := lancamentoSalvo(t, a, "ATACADAO", func(l *lancamento.Lancamento) {
		*l, _ = l.ComCategoria(1, lancamento.CategoriaPorRegra)
	})
	a.eventos.pendentes = []evento.Evento{eventoDe(t, l3)}
	if _, err := a.fila.ProcessarLote(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(a.mensageiro.avisos) != 2 || !strings.Contains(a.mensageiro.avisos[1], "100%") {
		t.Errorf("avisos = %v", a.mensageiro.avisos)
	}
}

func TestSalvarRecebeEvento(t *testing.T) {
	repo := &lancamentosDaFila{}
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
}

var _ RepositorioDeLancamentos = (*lancamentosDaFila)(nil)
