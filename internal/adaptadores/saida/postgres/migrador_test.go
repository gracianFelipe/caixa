package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gracianFelipe/caixa/migracoes"
)

func TestLerMigracoesOrdenaPorVersao(t *testing.T) {
	// MapFS: um fs.FS inventado na hora, sem tocar o disco.
	arquivos := fstest.MapFS{
		"010_dez.sql":     {Data: []byte("-- dez")},
		"002_dois.sql":    {Data: []byte("-- dois")},
		"001_um.sql":      {Data: []byte("-- um")},
		"README.md":       {Data: []byte("ignorado")},
		"rascunho.txt":    {Data: []byte("ignorado")},
		"pasta/003_x.sql": {Data: []byte("subpasta ignorada")},
	}

	obtidas, err := lerMigracoes(arquivos)
	if err != nil {
		t.Fatalf("lerMigracoes devolveu erro: %v", err)
	}

	queridas := []string{"001_um.sql", "002_dois.sql", "010_dez.sql"}
	if len(obtidas) != len(queridas) {
		t.Fatalf("leu %d migracoes, queria %d", len(obtidas), len(queridas))
	}
	for i, m := range obtidas {
		if m.nome != queridas[i] {
			t.Errorf("posicao %d: %s, queria %s", i, m.nome, queridas[i])
		}
	}
	if obtidas[2].versao != 10 || obtidas[2].sql != "-- dez" {
		t.Errorf("010 leu versao %d e sql %q", obtidas[2].versao, obtidas[2].sql)
	}
}

func TestLerMigracoesErro(t *testing.T) {
	casos := []struct {
		nome     string
		arquivos fstest.MapFS
		erro     error
	}{
		{"sem zero a esquerda", fstest.MapFS{"1_x.sql": {}}, ErrNomeDeMigracao},
		{"quatro digitos", fstest.MapFS{"0001_x.sql": {}}, ErrNomeDeMigracao},
		{"letras na versao", fstest.MapFS{"abc_x.sql": {}}, ErrNomeDeMigracao},
		{"sinal na versao", fstest.MapFS{"+01_x.sql": {}}, ErrNomeDeMigracao},
		{"versao zero", fstest.MapFS{"000_x.sql": {}}, ErrNomeDeMigracao},
		{"sem separador", fstest.MapFS{"001.sql": {}}, ErrNomeDeMigracao},
		{"sem nome", fstest.MapFS{"001_.sql": {}}, ErrNomeDeMigracao},
		{"versao repetida", fstest.MapFS{"001_a.sql": {}, "001_b.sql": {}}, ErrVersaoRepetida},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := lerMigracoes(c.arquivos); !errors.Is(err, c.erro) {
				t.Errorf("lerMigracoes devolveu %v, queria %v", err, c.erro)
			}
		})
	}
}

// TestMigracoesDoRepositorio le os arquivos embutidos de verdade: garante que
// ninguem commita um nome fora do padrao ou uma versao repetida.
func TestMigracoesDoRepositorio(t *testing.T) {
	ms, err := lerMigracoes(migracoes.Arquivos)
	if err != nil {
		t.Fatalf("migracoes embutidas invalidas: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("nenhuma migracao embutida")
	}
	for i, m := range ms {
		if m.versao != i+1 {
			t.Errorf("versoes precisam ser contiguas a partir de 001: posicao %d tem %s", i, m.nome)
		}
		if strings.TrimSpace(m.sql) == "" {
			t.Errorf("%s esta vazia", m.nome)
		}
	}
}

// TestConectarNaoVazaSenha: o erro de conexao sobe ate o log do cmd; a senha
// da URL nao pode ir junto.
func TestConectarNaoVazaSenha(t *testing.T) {
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()

	_, err := Conectar(ctx, "postgres://usuario:segredo-que-nao-vaza@127.0.0.1:1/caixa")
	if err == nil {
		t.Fatal("conectar na porta 1 deveria falhar")
	}
	if strings.Contains(err.Error(), "segredo-que-nao-vaza") {
		t.Errorf("erro de conexao contem a senha: %v", err)
	}
}
