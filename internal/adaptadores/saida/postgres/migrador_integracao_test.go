//go:build integracao

package postgres

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gracianFelipe/caixa/migracoes"
)

// TestAplicarEhIdempotente: rodar duas vezes deixa o banco igual e a segunda
// execucao nao aplica nada. E a propriedade que permite chamar `migrar` em
// todo deploy sem pensar.
func TestAplicarEhIdempotente(t *testing.T) {
	repo := repositorioDeTeste(t)
	ctx := context.Background()

	if _, err := Aplicar(ctx, repo.pool, migracoes.Arquivos); err != nil {
		t.Fatalf("primeira execucao: %v", err)
	}
	segunda, err := Aplicar(ctx, repo.pool, migracoes.Arquivos)
	if err != nil {
		t.Fatalf("segunda execucao: %v", err)
	}
	if len(segunda) != 0 {
		t.Errorf("segunda execucao aplicou %d migracoes, queria 0: %v", len(segunda), segunda)
	}

	// A tabela de controle tem exatamente as versoes embutidas.
	embutidas, _ := lerMigracoes(migracoes.Arquivos)
	var registradas int
	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM migracoes_aplicadas").Scan(&registradas); err != nil {
		t.Fatal(err)
	}
	if registradas != len(embutidas) {
		t.Errorf("migracoes_aplicadas tem %d linhas, queria %d", registradas, len(embutidas))
	}
}

// TestAplicarFalhaNaoDeixaRastro: migracao que quebra no meio nao cria a
// tabela nem se registra como aplicada.
func TestAplicarFalhaNaoDeixaRastro(t *testing.T) {
	repo := repositorioDeTeste(t)
	ctx := context.Background()

	quebrada := fstest.MapFS{
		"900_falha_de_teste.sql": {Data: []byte(`
			CREATE TABLE teste_rollback_002 (id INT);
			SELECT 1/0;
		`)},
	}
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, "DROP TABLE IF EXISTS teste_rollback_002")
		_, _ = repo.pool.Exec(ctx, "DELETE FROM migracoes_aplicadas WHERE versao = 900")
	})

	aplicadas, err := Aplicar(ctx, repo.pool, quebrada)
	if err == nil {
		t.Fatal("migracao com divisao por zero deveria falhar")
	}
	if !strings.Contains(err.Error(), "900_falha_de_teste.sql") {
		t.Errorf("erro nao identifica a migracao: %v", err)
	}
	if len(aplicadas) != 0 {
		t.Errorf("nada deveria constar como aplicado, veio %v", aplicadas)
	}

	var existe bool
	if err := repo.pool.QueryRow(ctx, "SELECT to_regclass('teste_rollback_002') IS NOT NULL").Scan(&existe); err != nil {
		t.Fatal(err)
	}
	if existe {
		t.Error("tabela da migracao quebrada existe: o rollback nao aconteceu")
	}

	var registrada int
	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM migracoes_aplicadas WHERE versao = $1", 900).Scan(&registrada); err != nil {
		t.Fatal(err)
	}
	if registrada != 0 {
		t.Error("migracao quebrada foi registrada como aplicada")
	}
}
