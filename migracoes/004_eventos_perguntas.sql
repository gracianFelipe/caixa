-- 004: outbox transacional e perguntas abertas no Telegram.

CREATE TABLE eventos (
    id           UUID        PRIMARY KEY,
    tipo         TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    criado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
    processado_em TIMESTAMPTZ
);

-- O worker so enxerga pendentes; indice parcial faz a fila ser varredura minima.
CREATE INDEX eventos_pendentes_idx ON eventos (criado_em)
    WHERE processado_em IS NULL;

CREATE TABLE perguntas (
    id            UUID        PRIMARY KEY,
    lancamento_id UUID        NOT NULL REFERENCES lancamentos (id),
    chat_id       BIGINT      NOT NULL,
    mensagem_id   BIGINT,
    estado        TEXT        NOT NULL DEFAULT 'aberta' CHECK (estado IN ('aberta','respondida','expirada')),
    criada_em     TIMESTAMPTZ NOT NULL DEFAULT now(),
    respondida_em TIMESTAMPTZ,

    -- Uma pergunta por lancamento: nao se pergunta duas vezes.
    UNIQUE (lancamento_id)
);
