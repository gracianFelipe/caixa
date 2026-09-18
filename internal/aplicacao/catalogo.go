package aplicacao

import (
	"context"
	"fmt"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
)

// Catalogo expoe o vocabulario fixo do sistema (hoje, so categorias).
type Catalogo struct {
	categorias RepositorioDeCategorias
}

func NovoCatalogo(categorias RepositorioDeCategorias) *Catalogo {
	return &Catalogo{categorias: categorias}
}

func (c *Catalogo) Categorias(ctx context.Context) ([]categoria.Categoria, error) {
	categorias, err := c.categorias.Listar(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando categorias: %w", err)
	}
	return categorias, nil
}
