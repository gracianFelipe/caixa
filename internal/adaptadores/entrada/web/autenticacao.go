package web

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Acesso e o que o handler precisa do login — interface no consumidor;
// *aplicacao.Acesso satisfaz.
type Acesso interface {
	Configurado() bool
	Entrar(ctx context.Context, usuario, senha string) (token string, err error)
	Validar(ctx context.Context, token string) (bool, error)
	Sair(ctx context.Context, token string) error
	Usuario() string
}

const nomeDoCookie = "caixa_sessao"

// cookieDeSessao monta o cookie com todas as defesas. MaxAge acompanha a
// duracao da sessao no servidor; quem manda e sempre o servidor (a sessao
// desliza la, o cookie so precisa durar pelo menos o mesmo tanto).
func cookieDeSessao(token string, seguro bool) *http.Cookie {
	return &http.Cookie{
		Name:     nomeDoCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   seguro,
		SameSite: http.SameSiteStrictMode,
	}
}

func cookieDeSaida(seguro bool) *http.Cookie {
	c := cookieDeSessao("", seguro)
	c.MaxAge = -1
	return c
}

// exigirSessao barra quem nao tem sessao viva. 401 seco em JSON: o front
// interpreta e leva para o login.
func (s *servidor) exigirSessao(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(nomeDoCookie)
		if err != nil {
			responderJSON(w, http.StatusUnauthorized, respostaDeErro{Erro: "sessao ausente"})
			return
		}
		ok, err := s.acesso.Validar(r.Context(), cookie.Value)
		if err != nil {
			s.responderErro(w, r, err)
			return
		}
		if !ok {
			responderJSON(w, http.StatusUnauthorized, respostaDeErro{Erro: "sessao invalida"})
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

// conferirOrigem e defesa em profundidade sobre o SameSite=Strict: navegador
// que manda Origin/Referer precisa bater com o host. Cliente sem navegador
// (curl, Atalho) nao manda nenhum dos dois e passa.
func conferirOrigem(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			proximo.ServeHTTP(w, r)
			return
		}

		origem := r.Header.Get("Origin")
		if origem == "" {
			origem = r.Header.Get("Referer")
		}
		if origem != "" {
			u, err := url.Parse(origem)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				responderJSON(w, http.StatusForbidden, respostaDeErro{Erro: "origem nao permitida"})
				return
			}
		}
		proximo.ServeHTTP(w, r)
	})
}

// limitadorPorIP e um balde fixo por janela: N tentativas por minuto por IP.
// Em memoria porque ha UM processo de API; o custo de errar aqui e so
// atrasar um atacante a menos — e o argon2 ja custa ~100ms por tentativa.
type limitadorPorIP struct {
	mu      sync.Mutex
	janelas map[string]*janelaDeTentativas
	limite  int
	duracao time.Duration
	agora   func() time.Time
}

type janelaDeTentativas struct {
	inicio time.Time
	usos   int
}

func novoLimitadorPorIP(limite int, duracao time.Duration, agora func() time.Time) *limitadorPorIP {
	return &limitadorPorIP{
		janelas: make(map[string]*janelaDeTentativas),
		limite:  limite, duracao: duracao, agora: agora,
	}
}

// Permite consome uma tentativa do IP e diz se ainda cabe. Janelas velhas
// sao recolhidas de passagem — sem goroutine de limpeza.
func (l *limitadorPorIP) Permite(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	agora := l.agora()
	if len(l.janelas) > 10_000 { // teto de memoria sob flood de IPs forjados
		for chave, j := range l.janelas {
			if agora.Sub(j.inicio) > l.duracao {
				delete(l.janelas, chave)
			}
		}
	}

	j := l.janelas[ip]
	if j == nil || agora.Sub(j.inicio) > l.duracao {
		l.janelas[ip] = &janelaDeTentativas{inicio: agora, usos: 1}
		return true
	}
	j.usos++
	return j.usos <= l.limite
}

func ipDoPedido(r *http.Request) string {
	// Sem confiar em X-Forwarded-For: quando houver proxy (Fase 8), o Caddy
	// fala com a API por loopback e o cabecalho seria forjavel por qualquer um.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type pedidoDeLogin struct {
	Usuario string `json:"usuario"`
	Senha   string `json:"senha"`
}

func (s *servidor) entrar(w http.ResponseWriter, r *http.Request) {
	if !s.acesso.Configurado() {
		responderJSON(w, http.StatusServiceUnavailable, respostaDeErro{Erro: "login nao configurado"})
		return
	}
	if !s.limitador.Permite(ipDoPedido(r)) {
		responderJSON(w, http.StatusTooManyRequests, respostaDeErro{Erro: "muitas tentativas"})
		return
	}

	var pedido pedidoDeLogin
	if err := lerJSON(w, r, &pedido); err != nil {
		s.responderErro(w, r, err)
		return
	}

	token, err := s.acesso.Entrar(r.Context(), pedido.Usuario, pedido.Senha)
	if err != nil {
		// Mensagem unica para usuario e senha errados: sem oraculo.
		responderJSON(w, http.StatusUnauthorized, respostaDeErro{Erro: "usuario ou senha invalidos"})
		return
	}

	http.SetCookie(w, cookieDeSessao(token, s.cookieSeguro))
	w.WriteHeader(http.StatusNoContent)
}

func (s *servidor) sessao(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(nomeDoCookie)
	if err != nil {
		responderJSON(w, http.StatusUnauthorized, respostaDeErro{Erro: "sessao ausente"})
		return
	}
	ok, err := s.acesso.Validar(r.Context(), cookie.Value)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}
	if !ok {
		responderJSON(w, http.StatusUnauthorized, respostaDeErro{Erro: "sessao invalida"})
		return
	}
	responderJSON(w, http.StatusOK, map[string]string{"usuario": s.acesso.Usuario()})
}

func (s *servidor) sair(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(nomeDoCookie); err == nil {
		if err := s.acesso.Sair(r.Context(), cookie.Value); err != nil {
			s.responderErro(w, r, err)
			return
		}
	}
	http.SetCookie(w, cookieDeSaida(s.cookieSeguro))
	w.WriteHeader(http.StatusNoContent)
}
