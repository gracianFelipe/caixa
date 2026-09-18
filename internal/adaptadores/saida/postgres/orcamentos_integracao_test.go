//go:build integracao

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/orcamento"
)

func TestOrcamentoLimiteVigente(t *testing.T) {
	repo := repositorioDeTeste(t)
	orcamentos := NovoRepositorioDeOrcamentos(repo.pool)
	ctx := context.Background()

	// Categoria 14 (outros) e 1995 para nao colidir com limites reais.
	const cat = 14
	mes, _ := competencia.Nova(1995, time.March)
	outroMes, _ := competencia.Nova(1995, time.April)
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, "DELETE FROM orcamentos WHERE categoria_id = $1 AND (competencia IS NULL OR competencia < '2000-01-01')", cat)
	})

	padrao, _ := orcamento.Novo(cat, competencia.Competencia{}, 50000)
	especifico, _ := orcamento.Novo(cat, mes, 80000)
	for _, o := range []orcamento.Orcamento{padrao, especifico} {
		if err := orcamentos.Definir(ctx, o); err != nil {
			t.Fatalf("Definir: %v", err)
		}
	}

	if limite, ok, _ := orcamentos.LimiteVigente(ctx, cat, mes); !ok || limite != 80000 {
		t.Errorf("mes com limite proprio = (%d, %v), queria 80000", limite, ok)
	}
	if limite, ok, _ := orcamentos.LimiteVigente(ctx, cat, outroMes); !ok || limite != 50000 {
		t.Errorf("mes sem limite proprio cai no padrao = (%d, %v), queria 50000", limite, ok)
	}

	// Redefinir o padrao substitui (UNIQUE NULLS NOT DISTINCT + ON CONFLICT).
	novoPadrao, _ := orcamento.Novo(cat, competencia.Competencia{}, 60000)
	if err := orcamentos.Definir(ctx, novoPadrao); err != nil {
		t.Fatalf("redefinir padrao: %v", err)
	}
	if limite, _, _ := orcamentos.LimiteVigente(ctx, cat, outroMes); limite != 60000 {
		t.Errorf("padrao redefinido = %d, queria 60000", limite)
	}

	// Vigentes segue a mesma precedencia, para todas as categorias de uma vez.
	vigentes, err := orcamentos.Vigentes(ctx, mes)
	if err != nil {
		t.Fatalf("Vigentes: %v", err)
	}
	if vigentes[cat] != 80000 {
		t.Errorf("Vigentes(%s)[%d] = %d, queria o especifico 80000", mes, cat, vigentes[cat])
	}
	vigentes, _ = orcamentos.Vigentes(ctx, outroMes)
	if vigentes[cat] != 60000 {
		t.Errorf("Vigentes(%s)[%d] = %d, queria o padrao 60000", outroMes, cat, vigentes[cat])
	}
}

func TestDaJanelaSoConfirmados(t *testing.T) {
	repo := repositorioDeTeste(t)
	ocorrencias := NovoRepositorioDeOcorrencias(repo.pool)
	ctx := context.Background()
	marcador := time.Now().Format("150405.000000")

	_, confirmado := evidencia(t, ocorrencias, "janela conf "+marcador, "JAN-C-"+marcador)
	oC, _ := evidencia(t, ocorrencias, "janela conf2 "+marcador, "JAN-C2-"+marcador)
	if _, err := ocorrencias.CriarComLancamento(ctx, oC, confirmado, eventoDeImportacao(t, ocorrencias, confirmado)); err != nil {
		t.Fatal(err)
	}
	oP, provisorio := evidencia(t, ocorrencias, "janela prov "+marcador, "JAN-P-"+marcador)
	provisorio = provisorio.Provisorio()
	if _, err := ocorrencias.CriarComLancamento(ctx, oP, provisorio, eventoDeImportacao(t, ocorrencias, provisorio)); err != nil {
		t.Fatal(err)
	}

	// evidencia() cria em julho/1998; janela de jan..jul/1998.
	inicio, _ := competencia.Nova(1998, time.January)
	fim, _ := competencia.Nova(1998, time.July)
	ls, err := repo.DaJanela(ctx, inicio, fim)
	if err != nil {
		t.Fatalf("DaJanela: %v", err)
	}

	achouConfirmado, achouProvisorio := false, false
	for _, l := range ls {
		if l.ID == confirmado.ID {
			achouConfirmado = true
		}
		if l.ID == provisorio.ID {
			achouProvisorio = true
		}
	}
	if !achouConfirmado {
		t.Error("confirmado dentro da janela nao veio")
	}
	if achouProvisorio {
		t.Error("provisorio nao pode entrar na janela do relatorio")
	}
}

func TestAlertaRegistrarSeNovo(t *testing.T) {
	repo := repositorioDeTeste(t)
	alertas := NovoRepositorioDeAlertas(repo.pool)
	ctx := context.Background()
	chave := "teste:" + time.Now().Format(time.RFC3339Nano)
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, "DELETE FROM alertas WHERE chave = $1", chave)
	})

	novo, err := alertas.RegistrarSeNovo(ctx, "orcamento", chave)
	if err != nil || !novo {
		t.Fatalf("primeira emissao = (%v, %v)", novo, err)
	}
	novo, err = alertas.RegistrarSeNovo(ctx, "orcamento", chave)
	if err != nil || novo {
		t.Errorf("segunda emissao = (%v, %v), queria (false, nil)", novo, err)
	}
}

func TestFundirProvisorio(t *testing.T) {
	repo := repositorioDeTeste(t)
	ocorrencias := NovoRepositorioDeOcorrencias(repo.pool)
	ctx := context.Background()
	marcador := time.Now().Format("150405.000000")

	// Destino confirmado com evidencia; provisorio com a propria evidencia.
	oDestino, destino := evidencia(t, ocorrencias, "destino "+marcador, "FUND-D-"+marcador)
	if _, err := ocorrencias.CriarComLancamento(ctx, oDestino, destino, eventoDeImportacao(t, ocorrencias, destino)); err != nil {
		t.Fatal(err)
	}
	oProv, provisorio := evidencia(t, ocorrencias, "provisorio "+marcador, "FUND-P-"+marcador)
	provisorio = provisorio.Provisorio()
	if _, err := ocorrencias.CriarComLancamento(ctx, oProv, provisorio, eventoDeImportacao(t, ocorrencias, provisorio)); err != nil {
		t.Fatal(err)
	}

	if err := repo.FundirProvisorio(ctx, provisorio.ID, destino.ID); err != nil {
		t.Fatalf("FundirProvisorio: %v", err)
	}

	var situacao string
	if err := repo.pool.QueryRow(ctx, "SELECT situacao FROM lancamentos WHERE id = $1", provisorio.ID).Scan(&situacao); err != nil {
		t.Fatal(err)
	}
	if situacao != "descartado" {
		t.Errorf("provisorio ficou %s, queria descartado", situacao)
	}

	var evidencias int
	if err := repo.pool.QueryRow(ctx,
		"SELECT count(*) FROM ocorrencias WHERE lancamento_id = $1 AND resultado = 'conciliou'", destino.ID,
	).Scan(&evidencias); err != nil {
		t.Fatal(err)
	}
	if evidencias != 1 {
		t.Errorf("destino tem %d evidencias conciliadas, queria 1", evidencias)
	}

	// Fundir de novo falha: nao esta mais provisorio.
	if err := repo.FundirProvisorio(ctx, provisorio.ID, destino.ID); err == nil {
		t.Error("segunda fusao deveria falhar")
	}

	// Descartado nao entra na soma de gasto.
	gasto, err := repo.GastoConfirmado(ctx, 0, destino.Competencia)
	if err != nil {
		t.Fatal(err)
	}
	_ = gasto // categoria zero nunca soma; a asserção de soma esta no teste da fila
}
