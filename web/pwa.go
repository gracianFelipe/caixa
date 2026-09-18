// Package web embute o PWA no binario. So embed: quem serve e o adaptador
// HTTP, que recebe este fs.FS e nao precisa saber de onde ele veio.
package web

import (
	"embed"
	"io/fs"
)

// arquivos carrega tudo de app/ (inclusive subpastas; "all:" traz arquivos
// que comecam com ponto ou sublinhado, se houver).
//
//go:embed all:app
var arquivos embed.FS

// App e o sistema de arquivos com a raiz em app/: index.html fica em "/".
func App() fs.FS {
	sub, err := fs.Sub(arquivos, "app")
	if err != nil {
		// So acontece se a pasta nao existir no embed — erro de build, nao de runtime.
		panic(err)
	}
	return sub
}
