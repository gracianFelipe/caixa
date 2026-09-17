// Package migracoes embute os arquivos SQL de migracao no binario.
// A diretiva go:embed copia os arquivos para dentro do executavel em tempo de
// compilacao; em runtime nao existe pasta migracoes/ para esquecer no deploy.
package migracoes

import "embed"

// Arquivos e o sistema de arquivos somente-leitura com todo *.sql desta pasta.
// O padrao so aceita arquivos deste diretorio; subpastas nao entram.
//
//go:embed *.sql
var Arquivos embed.FS
