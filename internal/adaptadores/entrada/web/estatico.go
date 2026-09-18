package web

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// servirApp serve o PWA embutido com fallback de SPA: caminho sem extensao
// (uma rota do app, como /lancamentos) devolve index.html para o roteador do
// front decidir; caminho com extensao que nao existe e 404 de verdade — um
// asset quebrado nao pode virar uma pagina HTML silenciosa.
func servirApp(arquivos fs.FS) http.Handler {
	servidorDeArquivos := http.FileServerFS(arquivos)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limpo := path.Clean("/" + r.URL.Path)

		if limpo == "/" || limpo == "/index.html" {
			servirIndex(w, r, arquivos)
			return
		}

		existe := false
		if info, err := fs.Stat(arquivos, strings.TrimPrefix(limpo, "/")); err == nil && !info.IsDir() {
			existe = true
		}

		switch {
		case existe:
			// index.html e o sw nao podem ficar presos em cache HTTP: o
			// service worker controla o cache dos assets.
			if strings.HasSuffix(limpo, ".html") || limpo == "/sw.js" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			servidorDeArquivos.ServeHTTP(w, r)
		case path.Ext(limpo) == "":
			servirIndex(w, r, arquivos)
		default:
			http.NotFound(w, r)
		}
	})
}

func servirIndex(w http.ResponseWriter, r *http.Request, arquivos fs.FS) {
	conteudo, err := fs.ReadFile(arquivos, "index.html")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(conteudo)
}
