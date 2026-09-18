// Package telegram e um cliente minimo da Bot API, escrito sobre net/http.
// Sem biblioteca: o projeto usa quatro metodos, e o custo de entender uma
// dependencia inteira e maior que o de escrever 150 linhas com teste.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// respostaMaxima limita o corpo aceito da API (IV: entrada externa).
const respostaMaxima = 1 << 20

var ErrAPI = errors.New("telegram: a API recusou a chamada")

// Cliente fala com um bot. O token vive na URL base e NUNCA aparece em erro
// ou log: os erros deste pacote carregam so o metodo chamado.
type Cliente struct {
	http *http.Client
	base string
}

// NovoCliente monta o cliente para api.telegram.org. baseURL parametrizavel
// existe para o teste apontar para um httptest.Server.
func NovoCliente(token string, baseURL string) *Cliente {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &Cliente{
		// Timeout acima dos 50s do long polling do getUpdates.
		http: &http.Client{Timeout: 65 * time.Second},
		base: baseURL + "/bot" + token,
	}
}

// Botao e uma opcao do teclado inline; Dados volta no callback (limite 64 bytes).
type Botao struct {
	Texto string `json:"text"`
	Dados string `json:"callback_data"`
}

// Mensagem e o retorno de sendMessage que interessa: o id para editar depois.
type Mensagem struct {
	ID int64 `json:"message_id"`
}

// Atualizacao e um item de getUpdates ja achatado para o que o worker usa.
type Atualizacao struct {
	ID         int64 // update_id, vira offset da proxima chamada
	ChatID     int64
	Texto      string // message.text; comandos como /relatorio chegam aqui
	Callback   string // callback_query.data; vazio quando nao e clique de botao
	CallbackID string
	MensagemID int64
}

// EnviarPergunta manda texto com botoes em linhas de ate 3 (limite pratico
// de largura no celular).
func (c *Cliente) EnviarPergunta(ctx context.Context, chatID int64, texto string, botoes []Botao) (Mensagem, error) {
	linhas := make([][]Botao, 0, (len(botoes)+2)/3)
	for len(botoes) > 0 {
		n := min(3, len(botoes))
		linhas = append(linhas, botoes[:n])
		botoes = botoes[n:]
	}

	var resposta struct {
		Resultado Mensagem `json:"result"`
	}
	err := c.chamar(ctx, "sendMessage", map[string]any{
		"chat_id":      chatID,
		"text":         texto,
		"reply_markup": map[string]any{"inline_keyboard": linhas},
	}, &resposta)
	return resposta.Resultado, err
}

// EnviarTexto manda uma mensagem simples, sem teclado.
func (c *Cliente) EnviarTexto(ctx context.Context, chatID int64, texto string) error {
	return c.chamar(ctx, "sendMessage", map[string]any{
		"chat_id": chatID,
		"text":    texto,
	}, nil)
}

// EditarMensagem troca o texto e remove o teclado (pergunta respondida).
func (c *Cliente) EditarMensagem(ctx context.Context, chatID, mensagemID int64, texto string) error {
	return c.chamar(ctx, "editMessageText", map[string]any{
		"chat_id":    chatID,
		"message_id": mensagemID,
		"text":       texto,
	}, nil)
}

// ConfirmarCallback fecha o "relogio" que o Telegram mostra no botao clicado.
func (c *Cliente) ConfirmarCallback(ctx context.Context, callbackID, texto string) error {
	return c.chamar(ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"text":              texto,
	}, nil)
}

// BuscarAtualizacoes faz long polling: bloqueia ate `timeout` segundos no
// servidor. offset = ultimo update_id + 1 confirma os anteriores.
func (c *Cliente) BuscarAtualizacoes(ctx context.Context, offset int64, timeoutSegundos int) ([]Atualizacao, error) {
	var resposta struct {
		Resultado []struct {
			UpdateID int64 `json:"update_id"`
			Callback *struct {
				ID       string `json:"id"`
				Dados    string `json:"data"`
				Mensagem *struct {
					ID   int64 `json:"message_id"`
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			} `json:"callback_query"`
			Mensagem *struct {
				Texto string `json:"text"`
				Chat  struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}

	err := c.chamar(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSegundos,
		"allowed_updates": []string{"message", "callback_query"},
	}, &resposta)
	if err != nil {
		return nil, err
	}

	atualizacoes := make([]Atualizacao, 0, len(resposta.Resultado))
	for _, r := range resposta.Resultado {
		a := Atualizacao{ID: r.UpdateID}
		switch {
		case r.Callback != nil && r.Callback.Mensagem != nil:
			a.ChatID = r.Callback.Mensagem.Chat.ID
			a.Callback = r.Callback.Dados
			a.CallbackID = r.Callback.ID
			a.MensagemID = r.Callback.Mensagem.ID
		case r.Mensagem != nil:
			a.ChatID = r.Mensagem.Chat.ID
			a.Texto = r.Mensagem.Texto
		}
		atualizacoes = append(atualizacoes, a)
	}
	return atualizacoes, nil
}

// chamar faz o POST JSON e decodifica. O corpo de erro da API nao vai para a
// mensagem de erro: poderia ecoar dados da conversa; fica so o metodo e o ok.
func (c *Cliente) chamar(ctx context.Context, metodo string, corpo any, destino any) error {
	dados, err := json.Marshal(corpo)
	if err != nil {
		return fmt.Errorf("telegram %s: serializando: %w", metodo, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+metodo, bytes.NewReader(dados))
	if err != nil {
		return fmt.Errorf("telegram %s: %w", metodo, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// O *url.Error do net/http carrega a URL inteira — e a URL tem o
		// token. Propaga-se so o erro de transporte interno.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("telegram %s: falha de rede: %w", metodo, err)
	}
	defer resp.Body.Close()

	leitor := io.LimitReader(resp.Body, respostaMaxima)
	var envelope struct {
		OK bool `json:"ok"`
	}
	bruto, err := io.ReadAll(leitor)
	if err != nil {
		return fmt.Errorf("telegram %s: lendo resposta: %w", metodo, err)
	}
	if err := json.Unmarshal(bruto, &envelope); err != nil || !envelope.OK {
		return fmt.Errorf("%w: %s (http %d)", ErrAPI, metodo, resp.StatusCode)
	}
	if destino != nil {
		if err := json.Unmarshal(bruto, destino); err != nil {
			return fmt.Errorf("telegram %s: decodificando: %w", metodo, err)
		}
	}
	return nil
}
