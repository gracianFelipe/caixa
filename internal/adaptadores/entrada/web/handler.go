// Package web expoe os casos de uso por HTTP/JSON usando so o net/http da
// stdlib. Padroes de rota "METODO /caminho" existem no ServeMux desde o Go 1.22.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gracianFelipe/caixa/internal/dominio/competencia"
	"github.com/gracianFelipe/caixa/internal/dominio/dinheiro"
	"github.com/gracianFelipe/caixa/internal/dominio/lancamento"
)

// Lancamentos e o que este adaptador precisa da aplicacao. Declarada aqui, no
// consumidor: *aplicacao.Lancamentos satisfaz sem saber que ela existe, e o
// teste usa um fake sem banco.
type Lancamentos interface {
	Registrar(ctx context.Context, d lancamento.Dados) (lancamento.Lancamento, error)
	Listar(ctx context.Context, c competencia.Competencia) ([]lancamento.Lancamento, error)
}

// Um lancamento em JSON tem ~200 bytes; 64 KiB e folga, nao permissao.
const corpoMaximo = 64 << 10

var (
	errCorpoGrande = errors.New("corpo acima do limite")
	errJSON        = errors.New("corpo invalido: esperado JSON com ocorrido_em, valor_centavos, meio e contraparte")
)

type servidor struct {
	lancamentos Lancamentos
	log         *slog.Logger
}

// NovoHandler monta as rotas e devolve http.Handler, nao *ServeMux: quem
// chama nao precisa saber como as rotas sao montadas.
func NovoHandler(l Lancamentos, log *slog.Logger) http.Handler {
	s := &servidor{lancamentos: l, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /saude", s.saude)
	mux.HandleFunc("POST /lancamentos", s.registrar)
	mux.HandleFunc("GET /lancamentos", s.listar)

	return registrarAcesso(log, mux)
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
}

type respostaDeErro struct {
	Erro string `json:"erro"`
}

func paraResposta(l lancamento.Lancamento) respostaDeLancamento {
	return respostaDeLancamento{
		ID:            l.ID.String(),
		OcorridoEm:    l.OcorridoEm,
		Competencia:   l.Competencia.String(),
		ValorCentavos: int64(l.Valor),
		Valor:         l.Valor.String(),
		Meio:          string(l.Meio),
		Contraparte:   l.Contraparte,
	}
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
	lancamento.ErrInstanteZero,
	lancamento.ErrValorZero,
	lancamento.ErrMeioInvalido,
	lancamento.ErrContraparteVazia,
	lancamento.ErrContraparteLonga,
}

func (s *servidor) responderErro(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errCorpoGrande) {
		responderJSON(w, http.StatusRequestEntityTooLarge, respostaDeErro{Erro: err.Error()})
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
