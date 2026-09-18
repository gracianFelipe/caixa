package aplicacao

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
	"github.com/gracianFelipe/caixa/internal/dominio/ocorrencia"
)

// ocorrenciasEmMemoria simula a constraint UNIQUE (origem, impressao) do
// banco: e o comportamento dela que o caso de uso depende.
type ocorrenciasEmMemoria struct {
	porImpressao map[string]bool
	criadas      []ocorrencia.Ocorrencia
	lancamentos  []lancamento.Lancamento
	falha        error
}

func novoFakeDeOcorrencias() *ocorrenciasEmMemoria {
	return &ocorrenciasEmMemoria{porImpressao: make(map[string]bool)}
}

func (r *ocorrenciasEmMemoria) CriarComLancamento(_ context.Context, o ocorrencia.Ocorrencia, l lancamento.Lancamento) (bool, error) {
	if r.falha != nil {
		return false, r.falha
	}
	chave := string(rune(o.Origem)) + o.Impressao
	if r.porImpressao[chave] {
		return false, nil
	}
	r.porImpressao[chave] = true
	r.criadas = append(r.criadas, o)
	r.lancamentos = append(r.lancamentos, l)
	return true, nil
}

func itemValido(payload string) ItemDeExtrato {
	return ItemDeExtrato{
		OcorridoEm:  time.Date(2026, time.September, 2, 13, 0, 0, 0, time.UTC),
		Valor:       -4790,
		Meio:        lancamento.MeioPix,
		Contraparte: "SUPERMERCADO SINTETICO",
		IDExterno:   "N0001",
		Payload:     payload,
	}
}

func TestImportar(t *testing.T) {
	repo := novoFakeDeOcorrencias()
	s := NovoServicoDeImportacao(repo, relogioFixo(time.Now()), saoPaulo)
	ctx := context.Background()

	itens := []ItemDeExtrato{
		itemValido("bloco A"),
		itemValido("bloco B"),
		{OcorridoEm: time.Now(), Valor: 0, Meio: lancamento.MeioPix, Contraparte: "SALDO", Payload: "saldo"},
	}

	resumo, err := s.Importar(ctx, ocorrencia.OrigemExtratoOFX, itens)
	if err != nil {
		t.Fatalf("Importar devolveu erro: %v", err)
	}
	if resumo != (ResumoDaImportacao{Criados: 2, Duplicados: 0, Ignorados: 1}) {
		t.Errorf("resumo = %+v", resumo)
	}

	// Reimportar o mesmo lote: tudo que foi criado vira duplicado.
	resumo, err = s.Importar(ctx, ocorrencia.OrigemExtratoOFX, itens)
	if err != nil {
		t.Fatal(err)
	}
	if resumo != (ResumoDaImportacao{Criados: 0, Duplicados: 2, Ignorados: 1}) {
		t.Errorf("resumo da reimportacao = %+v", resumo)
	}

	if len(repo.criadas) != 2 || len(repo.lancamentos) != 2 {
		t.Fatalf("fake tem %d ocorrencias e %d lancamentos, queria 2 e 2", len(repo.criadas), len(repo.lancamentos))
	}
	if repo.criadas[0].Resultado != ocorrencia.ResultadoPendente {
		t.Error("ocorrencia deveria chegar pendente ao repositorio")
	}
	if repo.lancamentos[0].Competencia.String() != "2026-09" {
		t.Errorf("competencia = %s", repo.lancamentos[0].Competencia)
	}
}

func TestImportarErro(t *testing.T) {
	ctx := context.Background()

	t.Run("item invalido aborta com posicao", func(t *testing.T) {
		s := NovoServicoDeImportacao(novoFakeDeOcorrencias(), relogioFixo(time.Now()), saoPaulo)
		ruim := itemValido("bloco")
		ruim.Contraparte = ""

		_, err := s.Importar(ctx, ocorrencia.OrigemExtratoOFX, []ItemDeExtrato{itemValido("ok"), ruim})
		if !errors.Is(err, lancamento.ErrContraparteVazia) {
			t.Fatalf("erro = %v, queria ErrContraparteVazia", err)
		}
		// A mensagem identifica a posicao, nunca o conteudo (PII).
		if want := "item 2"; !strings.Contains(err.Error(), want) {
			t.Errorf("erro %q nao identifica %q", err, want)
		}
	})

	t.Run("falha do repositorio interrompe", func(t *testing.T) {
		repo := novoFakeDeOcorrencias()
		repo.falha = errors.New("banco caiu")
		s := NovoServicoDeImportacao(repo, relogioFixo(time.Now()), saoPaulo)

		resumo, err := s.Importar(ctx, ocorrencia.OrigemExtratoOFX, []ItemDeExtrato{itemValido("x")})
		if !errors.Is(err, repo.falha) {
			t.Fatalf("erro = %v", err)
		}
		if resumo.Criados != 0 {
			t.Error("nada deveria constar como criado")
		}
	})

	t.Run("origem invalida", func(t *testing.T) {
		s := NovoServicoDeImportacao(novoFakeDeOcorrencias(), relogioFixo(time.Now()), saoPaulo)
		_, err := s.Importar(ctx, ocorrencia.Origem(99), []ItemDeExtrato{itemValido("x")})
		if !errors.Is(err, ocorrencia.ErrOrigemInvalida) {
			t.Fatalf("erro = %v, queria ErrOrigemInvalida", err)
		}
	})
}
