package aplicacao

import (
	"context"
	"errors"
	"testing"
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
)

var saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)

// relogioFixo devolve sempre o mesmo instante: teste nao depende do relogio da maquina.
type relogioFixo time.Time

func (r relogioFixo) Agora() time.Time { return time.Time(r) }

// regrasFixas implementa RepositorioDeRegras com uma lista em memoria.
type regrasFixas struct {
	regras []categorizacao.Regra
	falha  error
}

func (r regrasFixas) Ativas(context.Context) ([]categorizacao.Regra, error) {
	return r.regras, r.falha
}

func regraDeTeste(t *testing.T, id int64, cat categoria.ID, tipo categorizacao.Tipo, padrao string) categorizacao.Regra {
	t.Helper()
	regra, err := categorizacao.NovaRegra(id, cat, tipo, padrao, 0)
	if err != nil {
		t.Fatal(err)
	}
	return regra
}

// repoEmMemoria e um fake, nao um mock: implementa a porta de verdade, com
// comportamento observavel, em vez de gravar uma sequencia esperada de chamadas.
type repoEmMemoria struct {
	salvos     []lancamento.Lancamento
	eventos    []evento.Evento
	candidatos []conciliacao.Candidato
	falha      error
}

func (r *repoEmMemoria) Salvar(_ context.Context, l lancamento.Lancamento, e evento.Evento) error {
	if r.falha != nil {
		return r.falha
	}
	r.salvos = append(r.salvos, l)
	r.eventos = append(r.eventos, e)
	return nil
}

func (r *repoEmMemoria) PorID(_ context.Context, id identidade.ID) (lancamento.Lancamento, bool, error) {
	for _, l := range r.salvos {
		if l.ID == id {
			return l, true, nil
		}
	}
	return lancamento.Lancamento{}, false, nil
}

func (r *repoEmMemoria) AtribuirCategoria(context.Context, identidade.ID, categoria.ID, lancamento.OrigemDaCategoria) error {
	return nil
}

func (r *repoEmMemoria) CandidatosParaConciliacao(context.Context, dinheiro.Centavos, time.Time, ocorrencia.Origem) ([]conciliacao.Candidato, error) {
	return r.candidatos, nil
}

func (r *repoEmMemoria) GastoConfirmado(context.Context, categoria.ID, competencia.Competencia) (dinheiro.Centavos, error) {
	return 0, nil
}

func (r *repoEmMemoria) FundirProvisorio(context.Context, identidade.ID, identidade.ID) error {
	return nil
}
func (r *repoEmMemoria) ConfirmarProvisorio(context.Context, identidade.ID) error { return nil }

func (r *repoEmMemoria) DaCompetencia(_ context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error) {
	if r.falha != nil {
		return nil, r.falha
	}
	var out []lancamento.Lancamento
	for _, l := range r.salvos {
		if l.Competencia == c {
			out = append(out, l)
		}
	}
	return out, nil
}

func dadosValidos() lancamento.Dados {
	return lancamento.Dados{
		OcorridoEm:  time.Date(2026, time.September, 17, 15, 0, 0, 0, time.UTC),
		Valor:       -4790,
		Meio:        lancamento.MeioPix,
		Contraparte: "Supermercado XYZ",
	}
}

func TestRegistrar(t *testing.T) {
	repo := &repoEmMemoria{}
	agora := relogioFixo(time.Date(2026, time.September, 17, 18, 0, 0, 0, time.UTC))
	s := NovoServicoDeLancamentos(repo, regrasFixas{}, agora, saoPaulo)

	l, err := s.Registrar(context.Background(), dadosValidos())
	if err != nil {
		t.Fatalf("Registrar devolveu erro: %v", err)
	}

	if l.ID.EhZero() {
		t.Error("lancamento registrado sem id")
	}
	if len(repo.salvos) != 1 {
		t.Fatalf("repositorio tem %d lancamentos, queria 1", len(repo.salvos))
	}
	if repo.salvos[0].ID != l.ID {
		t.Error("o lancamento devolvido nao e o que foi salvo")
	}
	if c := l.Competencia.String(); c != "2026-09" {
		t.Errorf("competencia = %s, queria 2026-09", c)
	}
	if l.CategoriaOrigem != lancamento.CategoriaPendente {
		t.Errorf("sem regra aplicavel deveria ficar pendente, veio %s", l.CategoriaOrigem)
	}
}

func TestRegistrarClassifica(t *testing.T) {
	repo := &repoEmMemoria{}
	regras := regrasFixas{regras: []categorizacao.Regra{
		regraDeTeste(t, 7, 2, categorizacao.TipoContem, "IFOOD"),
	}}
	s := NovoServicoDeLancamentos(repo, regras, relogioFixo(time.Now()), saoPaulo)

	d := dadosValidos()
	d.Contraparte = "IFD*IFOOD RESTAURANTE 123"

	l, err := s.Registrar(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if l.CategoriaID != 2 || l.CategoriaOrigem != lancamento.CategoriaPorRegra {
		t.Errorf("categoria = (%d, %s), queria (2, regra)", l.CategoriaID, l.CategoriaOrigem)
	}
	if repo.salvos[0].CategoriaID != 2 {
		t.Error("a categoria nao chegou ao repositorio")
	}
}

func TestRegistrarFalhaAoCarregarRegras(t *testing.T) {
	falha := errors.New("tabela sumiu")
	s := NovoServicoDeLancamentos(&repoEmMemoria{}, regrasFixas{falha: falha}, relogioFixo(time.Now()), saoPaulo)

	if _, err := s.Registrar(context.Background(), dadosValidos()); !errors.Is(err, falha) {
		t.Errorf("erro = %v, queria a falha das regras (engolir deixaria tudo pendente em silencio)", err)
	}
}

func TestRegistrarErro(t *testing.T) {
	falhaDoBanco := errors.New("conexao recusada")

	casos := []struct {
		nome   string
		ajuste func(*lancamento.Dados)
		repo   *repoEmMemoria
		erro   error
	}{
		{"erro de dominio sobe intacto", func(d *lancamento.Dados) { d.Valor = 0 }, &repoEmMemoria{}, lancamento.ErrValorZero},
		{"meio invalido", func(d *lancamento.Dados) { d.Meio = "cheque" }, &repoEmMemoria{}, lancamento.ErrMeioInvalido},
		{"erro do repositorio sobe embrulhado", func(*lancamento.Dados) {}, &repoEmMemoria{falha: falhaDoBanco}, falhaDoBanco},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := NovoServicoDeLancamentos(c.repo, regrasFixas{}, relogioFixo(time.Now()), saoPaulo)
			d := dadosValidos()
			c.ajuste(&d)

			_, err := s.Registrar(context.Background(), d)
			if !errors.Is(err, c.erro) {
				t.Errorf("Registrar devolveu %v, queria %v", err, c.erro)
			}
			if len(c.repo.salvos) != 0 {
				t.Error("lancamento invalido nao pode chegar ao repositorio")
			}
		})
	}
}

func TestListar(t *testing.T) {
	repo := &repoEmMemoria{}
	s := NovoServicoDeLancamentos(repo, regrasFixas{}, relogioFixo(time.Now()), saoPaulo)
	ctx := context.Background()

	setembro := dadosValidos()
	outubro := dadosValidos()
	outubro.OcorridoEm = time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)

	for _, d := range []lancamento.Dados{setembro, outubro, setembro} {
		if _, err := s.Registrar(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	c, _ := competencia.Nova(2026, time.September)
	ls, err := s.Listar(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 2 {
		t.Errorf("Listar(2026-09) devolveu %d lancamentos, queria 2", len(ls))
	}

	repo.falha = errors.New("timeout")
	if _, err := s.Listar(ctx, c); !errors.Is(err, repo.falha) {
		t.Errorf("Listar deveria propagar o erro do repositorio, devolveu %v", err)
	}
}
