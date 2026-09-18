package aplicacao

import (
	"context"
	"fmt"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/orcamento"
)

// Orcamentos e o caso de uso da tela de orcamentos: uma linha por categoria
// com limite vigente e gasto do mes, e a definicao de limite.
type Orcamentos struct {
	consulta    ConsultaDeOrcamentos
	lancamentos RepositorioDeLancamentos
	categorias  RepositorioDeCategorias
}

func NovoServicoDeOrcamentos(consulta ConsultaDeOrcamentos, l RepositorioDeLancamentos, c RepositorioDeCategorias) *Orcamentos {
	return &Orcamentos{consulta: consulta, lancamentos: l, categorias: c}
}

// Visao monta a tela inteira: TODAS as categorias aparecem, com ou sem
// limite — orcamento que nao existe tambem e informacao.
func (s *Orcamentos) Visao(ctx context.Context, comp competencia.Competencia) ([]VisaoDeOrcamento, error) {
	categorias, err := s.categorias.Listar(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando categorias: %w", err)
	}
	limites, err := s.consulta.VigentesDetalhados(ctx, comp)
	if err != nil {
		return nil, fmt.Errorf("consultando limites: %w", err)
	}
	porCategoria := make(map[categoria.ID]LimiteVigenteDetalhado, len(limites))
	for _, l := range limites {
		porCategoria[l.Categoria] = l
	}

	visao := make([]VisaoDeOrcamento, 0, len(categorias))
	for _, c := range categorias {
		v := VisaoDeOrcamento{Categoria: c}
		if l, tem := porCategoria[c.ID]; tem {
			v.Limite = l.Limite
			v.Especifico = l.Especifico
		}
		gasto, err := s.lancamentos.GastoConfirmado(ctx, c.ID, comp)
		if err != nil {
			return nil, fmt.Errorf("somando gasto de %s: %w", c.Nome, err)
		}
		v.Gasto = gasto
		visao = append(visao, v)
	}
	return visao, nil
}

// Definir valida no dominio e grava. Competencia zero = limite padrao.
func (s *Orcamentos) Definir(ctx context.Context, cat categoria.ID, comp competencia.Competencia, limite dinheiro.Centavos) error {
	o, err := orcamento.Novo(cat, comp, limite)
	if err != nil {
		return err
	}
	if err := s.consulta.Definir(ctx, o); err != nil {
		return fmt.Errorf("definindo orcamento: %w", err)
	}
	return nil
}
