// Package web expoe os casos de uso por HTTP/JSON usando so o net/http da
// stdlib. Padroes de rota "METODO /caminho" existem no ServeMux desde o Go 1.22.
package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gracianFelipe/caixa/internal/aplicacao"
	"github.com/gracianFelipe/caixa/internal/dominio/categoria"
	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/identidade"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// Lancamentos e o que este adaptador precisa da aplicacao. Declarada aqui, no
// consumidor: *aplicacao.Lancamentos satisfaz sem saber que ela existe, e o
// teste usa um fake sem banco.
type Lancamentos interface {
	Registrar(ctx context.Context, d lancamento.Dados) (lancamento.Lancamento, error)
	Listar(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error)
	Capturar(ctx context.Context, valorTexto, contraparte, meioTexto string) (lancamento.Lancamento, error)
	Categorizar(ctx context.Context, id identidade.ID, cat categoria.ID) (lancamento.Lancamento, error)
}

// OrcamentosServico e a visao e a escrita de limites que a tela consome.
type OrcamentosServico interface {
	Visao(ctx context.Context, comp competencia.Competencia) ([]aplicacao.VisaoDeOrcamento, error)
	Definir(ctx context.Context, cat categoria.ID, comp competencia.Competencia, limite dinheiro.Centavos) error
}

// Catalogo e a segunda interface deste consumidor: separada de Lancamentos
// porque servicos diferentes a implementam, e o teste finge cada uma sozinha.
type Catalogo interface {
	Categorias(ctx context.Context) ([]categoria.Categoria, error)
}

// Relatorios gera o relatorio mensal pronto para apresentar.
type Relatorios interface {
	Gerar(ctx context.Context, alvo competencia.Competencia) (aplicacao.RelatorioPronto, error)
}

// Servicos agrupa o que o handler consome; struct nomeada em vez de uma
// fileira de parametros posicionais.
type Servicos struct {
	Lancamentos Lancamentos
	Catalogo    Catalogo
	Relatorios  Relatorios
	Orcamentos  OrcamentosServico
	Acesso      Acesso
	Hub         *Hub  // nil desliga o /api/ao-vivo
	App         fs.FS // nil desliga o PWA (so API)
	AtalhoToken string
	// CookieInseguro derruba o Secure do cookie para dev em http://localhost.
	CookieInseguro bool
}

// Um lancamento em JSON tem ~200 bytes; 64 KiB e folga, nao permissao.
const corpoMaximo = 64 << 10

var (
	errCorpoGrande = errors.New("corpo acima do limite")
	errJSON        = errors.New("corpo invalido: esperado JSON com ocorrido_em, valor_centavos, meio e contraparte")
)

type servidor struct {
	lancamentos  Lancamentos
	catalogo     Catalogo
	relatorios   Relatorios
	orcamentos   OrcamentosServico
	acesso       Acesso
	limitador    *limitadorPorIP
	cookieSeguro bool
	log          *slog.Logger
}

// NovoHandler monta as rotas e devolve http.Handler. Tudo em /api exige
// sessao, exceto: POST /api/sessao (e o login), /saude (liveness) e o
// atalho, que autentica por Bearer proprio. AtalhoToken vazio desliga a
// rota do atalho — sem token nao existe endpoint para proteger.
func NovoHandler(sv Servicos, log *slog.Logger) http.Handler {
	s := &servidor{
		lancamentos: sv.Lancamentos, catalogo: sv.Catalogo, relatorios: sv.Relatorios,
		orcamentos: sv.Orcamentos, acesso: sv.Acesso,
		limitador:    novoLimitadorPorIP(5, time.Minute, time.Now),
		cookieSeguro: !sv.CookieInseguro,
		log:          log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /saude", s.saude)

	if sv.Acesso != nil {
		mux.HandleFunc("POST /api/sessao", s.entrar)
		mux.HandleFunc("GET /api/sessao", s.sessao)
		mux.HandleFunc("DELETE /api/sessao", s.sair)
	}

	protegido := func(h http.HandlerFunc) http.Handler {
		if sv.Acesso == nil {
			return h // API sem login configurado (testes antigos): aberta
		}
		return s.exigirSessao(h)
	}
	mux.Handle("POST /api/lancamentos", protegido(s.registrar))
	mux.Handle("GET /api/lancamentos", protegido(s.listar))
	mux.Handle("PUT /api/lancamentos/{id}/categoria", protegido(s.categorizar))
	mux.Handle("GET /api/categorias", protegido(s.categorias))
	mux.Handle("GET /api/relatorio/{competencia}", protegido(s.relatorio))
	if sv.Orcamentos != nil {
		mux.Handle("GET /api/orcamentos", protegido(s.listarOrcamentos))
		mux.Handle("PUT /api/orcamentos", protegido(s.definirOrcamento))
	}
	if sv.Hub != nil {
		mux.Handle("GET /api/ao-vivo", protegido(sv.Hub.ServeHTTP))
	}

	if sv.AtalhoToken != "" {
		atalho := exigirBearer(sv.AtalhoToken, http.HandlerFunc(s.capturar))
		mux.Handle("POST /api/atalho/lancamentos", atalho)
		mux.Handle("POST /atalho/lancamentos", atalho) // alias da fase 3; some na proxima versao
	}

	if sv.App != nil {
		mux.Handle("/", servirApp(sv.App))
	}

	return registrarAcesso(log, cabecalhosDeSeguranca(conferirOrigem(mux)))
}

// categorizar aplica categoria manual a um lancamento existente.
func (s *servidor) categorizar(w http.ResponseWriter, r *http.Request) {
	id, err := identidade.Analisar(r.PathValue("id"))
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	var pedido struct {
		CategoriaID int16 `json:"categoria_id"`
	}
	if err := lerJSON(w, r, &pedido); err != nil {
		s.responderErro(w, r, err)
		return
	}

	l, err := s.lancamentos.Categorizar(r.Context(), id, categoria.ID(pedido.CategoriaID))
	if err != nil {
		s.responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, paraResposta(l))
}

type respostaDeOrcamento struct {
	CategoriaID int16  `json:"categoria_id"`
	Nome        string `json:"nome"`
	// Ponteiro: null quando a categoria nao tem limite — zero seria mentira.
	LimiteCentavos *int64 `json:"limite_centavos"`
	GastoCentavos  int64  `json:"gasto_centavos"`
	Especifico     bool   `json:"especifico"`
}

func (s *servidor) listarOrcamentos(w http.ResponseWriter, r *http.Request) {
	comp, err := competencia.Analisar(r.URL.Query().Get("competencia"))
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	visao, err := s.orcamentos.Visao(r.Context(), comp)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	resposta := make([]respostaDeOrcamento, 0, len(visao))
	for _, v := range visao {
		item := respostaDeOrcamento{
			CategoriaID: int16(v.Categoria.ID), Nome: v.Categoria.Nome,
			GastoCentavos: int64(v.Gasto), Especifico: v.Especifico,
		}
		if v.Limite > 0 {
			limite := int64(v.Limite)
			item.LimiteCentavos = &limite
		}
		resposta = append(resposta, item)
	}
	responderJSON(w, http.StatusOK, resposta)
}

func (s *servidor) definirOrcamento(w http.ResponseWriter, r *http.Request) {
	var pedido struct {
		CategoriaID    int16   `json:"categoria_id"`
		LimiteCentavos int64   `json:"limite_centavos"`
		Competencia    *string `json:"competencia"` // null = limite padrao
	}
	if err := lerJSON(w, r, &pedido); err != nil {
		s.responderErro(w, r, err)
		return
	}

	var comp competencia.Competencia
	if pedido.Competencia != nil && *pedido.Competencia != "" {
		var err error
		if comp, err = competencia.Analisar(*pedido.Competencia); err != nil {
			s.responderErro(w, r, err)
			return
		}
	}

	if err := s.orcamentos.Definir(r.Context(), categoria.ID(pedido.CategoriaID), comp, dinheiro.Centavos(pedido.LimiteCentavos)); err != nil {
		s.responderErro(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respostaDeRelatorio e o contrato JSON do relatorio: centavos inteiros e o
// texto formatado, com nomes resolvidos — o cliente nao precisa de segunda
// chamada para nomear categorias.
type respostaDeRelatorio struct {
	Competencia       string                 `json:"competencia"`
	TotalSaidas       int64                  `json:"total_saidas_centavos"`
	TotalEntradas     int64                  `json:"total_entradas_centavos"`
	Saldo             int64                  `json:"saldo_centavos"`
	SaidasMesAnterior int64                  `json:"saidas_mes_anterior_centavos"`
	PorCategoria      []respostaDeCategoria2 `json:"por_categoria"`
	Sinais            []respostaDeSinal      `json:"sinais"`
	Texto             string                 `json:"texto"`
}

type respostaDeCategoria2 struct {
	CategoriaID int16  `json:"categoria_id"`
	Nome        string `json:"nome"`
	Total       int64  `json:"total_centavos"`
	Quantidade  int    `json:"quantidade"`
}

type respostaDeSinal struct {
	Tipo        string `json:"tipo"`
	CategoriaID int16  `json:"categoria_id"`
	Nome        string `json:"nome"`
	Contraparte string `json:"contraparte,omitempty"`
	Valor       int64  `json:"valor_centavos"`
	Severidade  int    `json:"severidade"`
	Detalhe     string `json:"detalhe"`
}

// relatorio usa r.PathValue: o padrao "{competencia}" do ServeMux 1.22
// extrai o segmento sem biblioteca de rotas.
func (s *servidor) relatorio(w http.ResponseWriter, r *http.Request) {
	alvo, err := competencia.Analisar(r.PathValue("competencia"))
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	pronto, err := s.relatorios.Gerar(r.Context(), alvo)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	rel := pronto.Relatorio
	resp := respostaDeRelatorio{
		Competencia:       rel.Competencia.String(),
		TotalSaidas:       int64(rel.TotalSaidas),
		TotalEntradas:     int64(rel.TotalEntradas),
		Saldo:             int64(rel.Saldo),
		SaidasMesAnterior: int64(rel.SaidasMesAnterior),
		PorCategoria:      make([]respostaDeCategoria2, 0, len(rel.PorCategoria)),
		Sinais:            make([]respostaDeSinal, 0, len(rel.Sinais)),
		Texto:             pronto.Texto,
	}
	for _, c := range rel.PorCategoria {
		resp.PorCategoria = append(resp.PorCategoria, respostaDeCategoria2{
			CategoriaID: int16(c.Categoria), Nome: pronto.Nomes[c.Categoria],
			Total: int64(c.Total), Quantidade: c.Quantidade,
		})
	}
	for _, sn := range rel.Sinais {
		resp.Sinais = append(resp.Sinais, respostaDeSinal{
			Tipo: string(sn.Tipo), CategoriaID: int16(sn.Categoria), Nome: pronto.Nomes[sn.Categoria],
			Contraparte: sn.Contraparte, Valor: int64(sn.Valor), Severidade: sn.Severidade, Detalhe: sn.Detalhe,
		})
	}
	responderJSON(w, http.StatusOK, resp)
}

// exigirBearer compara o token em tempo constante. Os hashes igualam o
// tamanho antes da comparacao: ConstantTimeCompare com tamanhos diferentes
// devolve na hora e vazaria o comprimento do segredo.
func exigirBearer(token string, proximo http.Handler) http.Handler {
	esperado := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebido, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if ok {
			hash := sha256.Sum256([]byte(recebido))
			ok = subtle.ConstantTimeCompare(esperado[:], hash[:]) == 1
		}
		if !ok {
			// 401 seco: nao se explica autenticacao para quem errou o token.
			responderJSON(w, http.StatusUnauthorized, respostaDeErro{Erro: "nao autorizado"})
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

// pedidoDeAtalho e a captura rapida do iOS: valor em texto brasileiro.
type pedidoDeAtalho struct {
	Valor       string `json:"valor"`
	Contraparte string `json:"contraparte"`
	Meio        string `json:"meio"`
}

func (s *servidor) capturar(w http.ResponseWriter, r *http.Request) {
	var pedido pedidoDeAtalho
	if err := lerJSON(w, r, &pedido); err != nil {
		s.responderErro(w, r, err)
		return
	}

	l, err := s.lancamentos.Capturar(r.Context(), pedido.Valor, pedido.Contraparte, pedido.Meio)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusCreated, paraResposta(l))
}

// pedidoDeLancamento e o contrato de entrada. valor_centavos e int64: o
// decoder recusa 47.90 por tipo, entao ponto flutuante nem chega ao dominio.
type pedidoDeLancamento struct {
	OcorridoEm    time.Time `json:"ocorrido_em"`
	ValorCentavos int64     `json:"valor_centavos"`
	Meio          string    `json:"meio"`
	Contraparte   string    `json:"contraparte"`
}

type respostaDeLancamento struct {
	ID            string    `json:"id"`
	OcorridoEm    time.Time `json:"ocorrido_em"`
	Competencia   string    `json:"competencia"`
	ValorCentavos int64     `json:"valor_centavos"`
	Valor         string    `json:"valor"`
	Meio          string    `json:"meio"`
	Contraparte   string    `json:"contraparte"`
	// Ponteiro para o JSON dizer null quando pendente: 0 seria um id falso.
	CategoriaID     *int16 `json:"categoria_id"`
	CategoriaOrigem string `json:"categoria_origem"`
}

type respostaDeCategoria struct {
	ID   int16  `json:"id"`
	Nome string `json:"nome"`
}

type respostaDeErro struct {
	Erro string `json:"erro"`
}

func paraResposta(l lancamento.Lancamento) respostaDeLancamento {
	r := respostaDeLancamento{
		ID:              l.ID.String(),
		OcorridoEm:      l.OcorridoEm,
		Competencia:     l.Competencia.String(),
		ValorCentavos:   int64(l.Valor),
		Valor:           l.Valor.String(),
		Meio:            string(l.Meio),
		Contraparte:     l.Contraparte,
		CategoriaOrigem: string(l.CategoriaOrigem),
	}
	if l.CategoriaID > 0 {
		id := int16(l.CategoriaID)
		r.CategoriaID = &id
	}
	return r
}

func (s *servidor) saude(w http.ResponseWriter, _ *http.Request) {
	responderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *servidor) registrar(w http.ResponseWriter, r *http.Request) {
	var pedido pedidoDeLancamento
	if err := lerJSON(w, r, &pedido); err != nil {
		s.responderErro(w, r, err)
		return
	}

	// Conversao direta, sem validar aqui: validar e papel de lancamento.Novo.
	l, err := s.lancamentos.Registrar(r.Context(), lancamento.Dados{
		OcorridoEm:  pedido.OcorridoEm,
		Valor:       dinheiro.Centavos(pedido.ValorCentavos),
		Meio:        lancamento.Meio(pedido.Meio),
		Contraparte: pedido.Contraparte,
	})
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	responderJSON(w, http.StatusCreated, paraResposta(l))
}

func (s *servidor) listar(w http.ResponseWriter, r *http.Request) {
	c, err := competencia.Analisar(r.URL.Query().Get("competencia"))
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	ls, err := s.lancamentos.Listar(r.Context(), c)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	// make com tamanho zero, nao var nil: slice vazia vira [] no JSON, nil vira null.
	resposta := make([]respostaDeLancamento, 0, len(ls))
	for _, l := range ls {
		resposta = append(resposta, paraResposta(l))
	}
	responderJSON(w, http.StatusOK, resposta)
}

func (s *servidor) categorias(w http.ResponseWriter, r *http.Request) {
	categorias, err := s.catalogo.Categorias(r.Context())
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	resposta := make([]respostaDeCategoria, 0, len(categorias))
	for _, c := range categorias {
		resposta = append(resposta, respostaDeCategoria{ID: int16(c.ID), Nome: c.Nome})
	}
	responderJSON(w, http.StatusOK, resposta)
}

// lerJSON decodifica o corpo com limite de tamanho, sem campos desconhecidos
// e sem conteudo depois do primeiro valor. Detalhe do decoder nao volta ao
// cliente: ele descreve tipos internos do programa.
func lerJSON(w http.ResponseWriter, r *http.Request, destino any) error {
	r.Body = http.MaxBytesReader(w, r.Body, corpoMaximo)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(destino); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			return errCorpoGrande
		}
		return errJSON
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errJSON
	}
	return nil
}

// errosDoCliente e a lista fechada do que vira 400. Qualquer erro fora dela
// e tratado como interno: mensagem generica ao cliente, detalhe so no log.
var errosDoCliente = []error{
	errJSON,
	competencia.ErrFormato,
	identidade.ErrFormato,
	identidade.ErrVazio,
	lancamento.ErrCategoriaInvalida,
	lancamento.ErrInstanteZero,
	lancamento.ErrValorZero,
	lancamento.ErrMeioInvalido,
	lancamento.ErrContraparteVazia,
	lancamento.ErrContraparteLonga,
	dinheiro.ErrVazio,
	dinheiro.ErrFormato,
	dinheiro.ErrEstouro,
}

func (s *servidor) responderErro(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errCorpoGrande) {
		responderJSON(w, http.StatusRequestEntityTooLarge, respostaDeErro{Erro: err.Error()})
		return
	}
	if errors.Is(err, aplicacao.ErrNaoEncontrado) {
		responderJSON(w, http.StatusNotFound, respostaDeErro{Erro: err.Error()})
		return
	}
	for _, conhecido := range errosDoCliente {
		if errors.Is(err, conhecido) {
			responderJSON(w, http.StatusBadRequest, respostaDeErro{Erro: err.Error()})
			return
		}
	}

	// O erro pode carregar o id do lancamento, nunca valor ou contraparte.
	s.log.ErrorContext(r.Context(), "erro interno",
		"metodo", r.Method, "rota", r.URL.Path, "erro", err)
	responderJSON(w, http.StatusInternalServerError, respostaDeErro{Erro: "erro interno"})
}

func responderJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Dado financeiro nao fica em cache de navegador nem de proxy.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// Falha aqui significa cliente que fechou a conexao; nao ha resposta alternativa.
	_ = json.NewEncoder(w).Encode(corpo)
}

// gravadorDeStatus intercepta WriteHeader para o log de acesso saber o status.
// Embutir http.ResponseWriter delega todos os outros metodos sem reescreve-los.
type gravadorDeStatus struct {
	http.ResponseWriter
	status int
}

func (g *gravadorDeStatus) WriteHeader(status int) {
	g.status = status
	g.ResponseWriter.WriteHeader(status)
}

// registrarAcesso loga metodo, rota, status e duracao. Query e corpo ficam de
// fora de proposito: podem carregar dado financeiro do usuario.
func registrarAcesso(log *slog.Logger, proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		g := &gravadorDeStatus{ResponseWriter: w, status: http.StatusOK}

		proximo.ServeHTTP(g, r)

		log.InfoContext(r.Context(), "requisicao",
			"metodo", r.Method, "rota", r.URL.Path, "status", g.status,
			"duracao_ms", time.Since(inicio).Milliseconds())
	})
}
