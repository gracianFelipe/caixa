//go:build integracao

package postgres

import (
	"context"
	"testing"

	"github.com/gracianFelipe/caixa/internal/dominio/categorizacao"
)

func TestRegrasAtivasSementes(t *testing.T) {
	repo := repositorioDeTeste(t)
	regras, err := NovoRepositorioDeRegras(repo.pool).Ativas(context.Background())
	if err != nil {
		t.Fatalf("Ativas: %v", err)
	}
	if len(regras) < 30 {
		t.Fatalf("esperava >= 30 regras semente, veio %d", len(regras))
	}

	// A semente IFOOD -> restaurante (2) precisa classificar de verdade.
	resultado, ok := categorizacao.Classificar("IFD IFOOD RESTAURANTE", regras)
	if !ok || resultado.Categoria != 2 {
		t.Errorf("Classificar(IFOOD) = (%+v, %v), queria categoria 2", resultado, ok)
	}

	// EXTRA e exata: nao pode casar dentro de EXTRATO.
	if _, ok := categorizacao.Classificar("EXTRATO BANCARIO", regras); ok {
		t.Error("EXTRATO nao deveria classificar (EXTRA e regra exata)")
	}
}

func TestCategoriasListar(t *testing.T) {
	repo := repositorioDeTeste(t)
	categorias, err := NovoRepositorioDeCategorias(repo.pool).Listar(context.Background())
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(categorias) != 14 {
		t.Fatalf("esperava 14 categorias semente, veio %d", len(categorias))
	}
	if categorias[0].ID != 1 || categorias[0].Nome != "mercado" {
		t.Errorf("primeira categoria = %+v, queria {1 mercado}", categorias[0])
	}
}
