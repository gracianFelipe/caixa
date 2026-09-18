// Package email busca mensagens novas por IMAP. Cola fina sobre go-imap/v2:
// conecta, seleciona INBOX, pede UIDs acima da ultima vista e devolve os
// .eml crus — quem entende o conteudo e o parser bradesco; quem persiste a
// posicao e o worker.
package email

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Mensagem e um e-mail cru com a UID que o servidor lhe deu.
type Mensagem struct {
	UID   uint32
	Bruto []byte
}

// Caixa descreve a conta consultada. A senha vive aqui em memoria e nunca
// em log — os erros deste pacote nao a carregam.
type Caixa struct {
	Servidor string // host:993, TLS implicito
	Usuario  string
	Senha    string
}

// Buscar devolve as mensagens com UID > ultimaUID e a UIDVALIDITY atual.
// Conexao por chamada: o worker consulta a cada 2 minutos e manter sessao
// IMAP viva atraves de NATs e timeouts custa mais do que reconectar.
func (c Caixa) Buscar(ctx context.Context, uidvalidity uint32, ultimaUID uint32) ([]Mensagem, uint32, error) {
	cliente, err := imapclient.DialTLS(c.Servidor, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("email: conectando: %w", err)
	}
	defer cliente.Close()

	if err := cliente.Login(c.Usuario, c.Senha).Wait(); err != nil {
		return nil, 0, fmt.Errorf("email: autenticando: %w", err)
	}
	defer cliente.Logout()

	caixa, err := cliente.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return nil, 0, fmt.Errorf("email: selecionando INBOX: %w", err)
	}

	// UIDVALIDITY mudou = o servidor renumerou tudo: a ultimaUID do banco
	// nao vale mais e o chamador recomeca (a idempotencia por Message-Id
	// impede reimportacao duplicada).
	if caixa.UIDValidity != uidvalidity {
		ultimaUID = 0
	}

	conjunto := imap.UIDSet{}
	conjunto.AddRange(imap.UID(ultimaUID+1), 0) // 0 = "*": tudo acima da ultima

	dados, err := cliente.Fetch(conjunto, &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}).Collect()
	if err != nil {
		return nil, 0, fmt.Errorf("email: buscando mensagens: %w", err)
	}

	mensagens := make([]Mensagem, 0, len(dados))
	for _, m := range dados {
		if uint32(m.UID) <= ultimaUID { // "*" devolve a ultima mesmo sem novas
			continue
		}
		var bruto []byte
		for _, secao := range m.BodySection {
			bruto = secao.Bytes
			break
		}
		if len(bruto) == 0 {
			continue
		}
		mensagens = append(mensagens, Mensagem{UID: uint32(m.UID), Bruto: bruto})
	}
	return mensagens, caixa.UIDValidity, nil
}
