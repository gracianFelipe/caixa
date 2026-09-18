package web

import "net/http"

// politicaDeConteudo e estrita porque o app nao tem script nem style inline:
// so o que o proprio binario serve. connect-src 'self' cobre o WebSocket da
// mesma origem; img-src data: permite icones SVG embutidos em CSS.
const politicaDeConteudo = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'; font-src 'self'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

// cabecalhosDeSeguranca aplica os cabecalhos que o ZAP procura (checklist
// secao 3.3). HSTS nao entra aqui: e do proxy TLS (Caddy, Fase 8) — mandar
// HSTS em texto puro no dev seria ruido.
func cabecalhosDeSeguranca(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", politicaDeConteudo)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		proximo.ServeHTTP(w, r)
	})
}
