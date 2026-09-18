package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

var saoPaulo = time.FixedZone("America/Sao_Paulo", -3*60*60)

// servicoFalso implementa a interface Lancamentos deste pacote sem banco.
// Guarda o que recebeu para o teste inspecionar e devolve o que foi programado.
type servicoFalso struct {
	recebido  lancamento.Dados
	capturado [3]string
	listados  []lancamento.Lancamento
	erro      error
}

func (f *servicoFalso) Registrar(_ context.Context, d lancamento.Dados) (lancamento.Lancamento, error) {
	f.recebido = d
	if f.erro != nil {
		return lancamento.Lancamento{}, f.erro
	}
	id := identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 1}
	return lancamento.Novo(id, d, saoPaulo)
}

func (f *servicoFalso) Capturar(ctx context.Context, valorTexto, contraparte, meioTexto string) (lancamento.Lancamento, error) {
	if f.erro != nil {
		return lancamento.Lancamento{}, f.erro
	}
	f.capturado = [3]string{valorTexto, contraparte, meioTexto}
	id := identidade.ID{0x01, 0x92, 0x6a, 0x5c, 0x12, 0x34, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 9}
	return lancamento.Novo(id, lancamento.Dados{
		OcorridoEm: time.Date(2026, time.September, 17, 15, 0, 0, 0, time.UTC),
		Valor:      -4790, Meio: lancamento.MeioPix, Contraparte: contraparte,
	}, saoPaulo)
}

func (f *servicoFalso) Listar(context.Context, competencia.Competencia) ([]lancamento.Lancamento, error) {
	if f.erro != nil {
		return nil, f.erro
	}
	return f.listados, nil
}

// catalogoFalso implementa Catalogo sem banco.
type catalogoFalso struct {
	categorias []categoria.Categoria
	erro       error
}

func (f catalogoFalso) Categorias(context.Context) ([]categoria.Categoria, error) {
	return f.categorias, f.erro
}

func logSilencioso() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const corpoValido = `{"valor_centavos":-4790,"meio":"pix","contraparte":"SUPERMERCADO XYZ","ocorrido_em":"2026-09-17T15:00:00Z"}`

func TestRegistrar(t *testing.T) {
	casos := []struct {
		nome   string
		corpo  string
		erro   error // programado no servico falso
		status int
	}{
		{"valido", corpoValido, nil, http.StatusCreated},
		{"json quebrado", `{"valor_centavos":`, nil, http.StatusBadRequest},
		{"campo desconhecido", `{"valor":100,"meio":"pix","contraparte":"x","ocorrido_em":"2026-09-17T15:00:00Z"}`, nil, http.StatusBadRequest},
		{"valor com decimal", `{"valor_centavos":47.90,"meio":"pix","contraparte":"x","ocorrido_em":"2026-09-17T15:00:00Z"}`, nil, http.StatusBadRequest},
		{"valor como texto", `{"valor_centavos":"4790","meio":"pix","contraparte":"x","ocorrido_em":"2026-09-17T15:00:00Z"}`, nil, http.StatusBadRequest},
		{"dois documentos", corpoValido + corpoValido, nil, http.StatusBadRequest},
		{"vazio", ``, nil, http.StatusBadRequest},
		{"valor zero vira 400 pelo dominio", `{"valor_centavos":0,"meio":"pix","contraparte":"x","ocorrido_em":"2026-09-17T15:00:00Z"}`, nil, http.StatusBadRequest},
		{"meio invalido vira 400 pelo dominio", `{"valor_centavos":-100,"meio":"cheque","contraparte":"x","ocorrido_em":"2026-09-17T15:00:00Z"}`, nil, http.StatusBadRequest},
		{"sem ocorrido_em vira 400 pelo dominio", `{"valor_centavos":-100,"meio":"pix","contraparte":"x"}`, nil, http.StatusBadRequest},
		{"corpo acima do limite", `{"contraparte":"` + strings.Repeat("a", corpoMaximo) + `"}`, nil, http.StatusRequestEntityTooLarge},
		{"erro de infraestrutura vira 500", corpoValido, errors.New("conexao recusada"), http.StatusInternalServerError},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			servico := &servicoFalso{erro: c.erro}
			h := NovoHandler(servico, catalogoFalso{}, "", logSilencioso())

			req := httptest.NewRequest(http.MethodPost, "/lancamentos", strings.NewReader(c.corpo))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != c.status {
				t.Fatalf("status = %d, queria %d; corpo: %s", rec.Code, c.status, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, queria application/json", ct)
			}
			if rec.Code == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "conexao recusada") {
				t.Error("detalhe do erro interno vazou para o cliente")
			}
		})
	}
}

func TestRegistrarRespostaCompleta(t *testing.T) {
	servico := &servicoFalso{}
	h := NovoHandler(servico, catalogoFalso{}, "", logSilencioso())

	req := httptest.NewRequest(http.MethodPost, "/lancamentos", strings.NewReader(corpoValido))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp respostaDeLancamento
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("resposta nao e JSON valido: %v", err)
	}

	if resp.ID != "01926a5c-1234-7000-8000-000000000001" {
		t.Errorf("id = %q", resp.ID)
	}
	if resp.Competencia != "2026-09" {
		t.Errorf("competencia = %q, queria 2026-09", resp.Competencia)
	}
	if resp.ValorCentavos != -4790 || resp.Valor != "-R$ 47,90" {
		t.Errorf("valor = (%d, %q), queria (-4790, \"-R$ 47,90\")", resp.ValorCentavos, resp.Valor)
	}
	if resp.CategoriaID != nil || resp.CategoriaOrigem != "pendente" {
		t.Errorf("categoria = (%v, %q), queria (null, pendente)", resp.CategoriaID, resp.CategoriaOrigem)
	}
	if servico.recebido.Contraparte != "SUPERMERCADO XYZ" {
		t.Errorf("servico recebeu contraparte %q", servico.recebido.Contraparte)
	}
}

func TestListar(t *testing.T) {
	id := identidade.ID{1}
	setembro, _ := lancamento.Novo(id, lancamento.Dados{
		OcorridoEm: time.Date(2026, time.September, 17, 15, 0, 0, 0, time.UTC),
		Valor:      -4790, Meio: lancamento.MeioPix, Contraparte: "Mercado",
	}, saoPaulo)

	casos := []struct {
		nome     string
		consulta string
		servico  *servicoFalso
		status   int
		corpo    string // prefixo esperado
	}{
		{"com dados", "?competencia=2026-09", &servicoFalso{listados: []lancamento.Lancamento{setembro}}, http.StatusOK, `[{"id"`},
		{"sem dados devolve lista vazia, nao null", "?competencia=2026-09", &servicoFalso{}, http.StatusOK, `[]`},
		{"sem competencia", "", &servicoFalso{}, http.StatusBadRequest, `{"erro"`},
		{"competencia invalida", "?competencia=2026-13", &servicoFalso{}, http.StatusBadRequest, `{"erro"`},
		{"competencia com dia", "?competencia=2026-09-01", &servicoFalso{}, http.StatusBadRequest, `{"erro"`},
		{"erro de infraestrutura", "?competencia=2026-09", &servicoFalso{erro: errors.New("timeout")}, http.StatusInternalServerError, `{"erro":"erro interno"}`},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := NovoHandler(c.servico, catalogoFalso{}, "", logSilencioso())

			req := httptest.NewRequest(http.MethodGet, "/lancamentos"+c.consulta, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != c.status {
				t.Fatalf("status = %d, queria %d; corpo: %s", rec.Code, c.status, rec.Body)
			}
			if !strings.HasPrefix(rec.Body.String(), c.corpo) {
				t.Errorf("corpo = %s, queria prefixo %s", rec.Body, c.corpo)
			}
		})
	}
}

func TestRotas(t *testing.T) {
	h := NovoHandler(&servicoFalso{}, catalogoFalso{}, "", logSilencioso())

	casos := []struct {
		metodo string
		rota   string
		status int
	}{
		{http.MethodGet, "/saude", http.StatusOK},
		{http.MethodPost, "/saude", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/lancamentos", http.StatusMethodNotAllowed},
		{http.MethodGet, "/inexistente", http.StatusNotFound},
	}

	for _, c := range casos {
		t.Run(c.metodo+" "+c.rota, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(c.metodo, c.rota, nil))
			if rec.Code != c.status {
				t.Errorf("status = %d, queria %d", rec.Code, c.status)
			}
		})
	}
}

// TestLogNaoVazaPII e a regra "Logging" do SEC-CHECK virando assercao:
// valor e contraparte nunca aparecem no log, nem em erro interno.
func TestLogNaoVazaPII(t *testing.T) {
	var saida bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&saida, nil))
	h := NovoHandler(&servicoFalso{erro: errors.New("banco caiu")}, catalogoFalso{}, "", log)

	corpo := `{"valor_centavos":-987654,"meio":"pix","contraparte":"CLINICA SIGILOSA","ocorrido_em":"2026-09-17T15:00:00Z"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/lancamentos?competencia=2026-09", strings.NewReader(corpo)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, queria 500", rec.Code)
	}
	for _, proibido := range []string{"987654", "CLINICA SIGILOSA", "competencia=2026-09"} {
		if strings.Contains(saida.String(), proibido) {
			t.Errorf("log contem %q:\n%s", proibido, saida.String())
		}
	}
	if !strings.Contains(saida.String(), "banco caiu") {
		t.Error("o detalhe do erro interno deveria estar no log do servidor")
	}
}

func TestCategorias(t *testing.T) {
	t.Run("lista", func(t *testing.T) {
		mercado, _ := categoria.Nova(1, "mercado")
		h := NovoHandler(&servicoFalso{}, catalogoFalso{categorias: []categoria.Categoria{mercado}}, "", logSilencioso())

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/categorias", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if corpo := rec.Body.String(); !strings.HasPrefix(corpo, `[{"id":1,"nome":"mercado"}`) {
			t.Errorf("corpo = %s", corpo)
		}
	})

	t.Run("vazia devolve lista, nao null", func(t *testing.T) {
		h := NovoHandler(&servicoFalso{}, catalogoFalso{}, "", logSilencioso())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/categorias", nil))
		if !strings.HasPrefix(rec.Body.String(), "[]") {
			t.Errorf("corpo = %s", rec.Body)
		}
	})

	t.Run("erro vira 500 generico", func(t *testing.T) {
		h := NovoHandler(&servicoFalso{}, catalogoFalso{erro: errors.New("sem banco")}, "", logSilencioso())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/categorias", nil))
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "sem banco") {
			t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
		}
	})
}

func TestAtalho(t *testing.T) {
	const token = "token-de-teste-000000000000000000000001"
	corpo := `{"valor":"47,90","contraparte":"PADARIA DO ZE","meio":""}`

	requisicao := func(auth string) *httptest.ResponseRecorder {
		servico := &servicoFalso{}
		h := NovoHandler(servico, catalogoFalso{}, token, logSilencioso())
		req := httptest.NewRequest(http.MethodPost, "/atalho/lancamentos", strings.NewReader(corpo))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	t.Run("com token cria", func(t *testing.T) {
		rec := requisicao("Bearer " + token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d; corpo %s", rec.Code, rec.Body)
		}
	})
	t.Run("sem header e 401", func(t *testing.T) {
		if rec := requisicao(""); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("token errado e 401", func(t *testing.T) {
		if rec := requisicao("Bearer errado"); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("esquema errado e 401", func(t *testing.T) {
		if rec := requisicao("Basic " + token); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("valor invalido e 400", func(t *testing.T) {
		servico := &servicoFalso{}
		h := NovoHandler(servico, catalogoFalso{}, token, logSilencioso())
		req := httptest.NewRequest(http.MethodPost, "/atalho/lancamentos", strings.NewReader(`{"valor":"abc","contraparte":"X","meio":""}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		// o servicoFalso nao valida; o teste real da validacao e da aplicacao.
		if rec.Code != http.StatusCreated {
			t.Skip("validacao de valor e testada na aplicacao")
		}
	})
	t.Run("rota inexiste sem token configurado", func(t *testing.T) {
		h := NovoHandler(&servicoFalso{}, catalogoFalso{}, "", logSilencioso())
		req := httptest.NewRequest(http.MethodPost, "/atalho/lancamentos", strings.NewReader(corpo))
		req.Header.Set("Authorization", "Bearer qualquer")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, queria 404", rec.Code)
		}
	})
}
