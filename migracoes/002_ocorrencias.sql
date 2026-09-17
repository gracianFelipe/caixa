-- 002: a evidencia. Cada mensagem/linha de extrato que chegou vira uma
-- ocorrencia; conciliar anexa uma segunda evidencia ao mesmo lancamento,
-- nunca apaga linha.

CREATE TABLE origens (
    id   SMALLINT PRIMARY KEY,
    nome TEXT     NOT NULL UNIQUE
);

INSERT INTO origens (id, nome) VALUES
    (1, 'manual'),
    (2, 'extrato_ofx'),
    (3, 'email_bradesco'),
    (4, 'telegram');

CREATE TABLE ocorrencias (
    id            UUID        PRIMARY KEY,
    origem_id     SMALLINT    NOT NULL REFERENCES origens (id),
    id_externo    TEXT,                                   -- FITID do OFX, Message-Id do e-mail
    impressao     TEXT        NOT NULL,                   -- sha256 da carga normalizada
    payload       JSONB       NOT NULL,                   -- bruto, valores como texto, nunca numero interpretado
    lancamento_id UUID        REFERENCES lancamentos (id),
    resultado     TEXT        NOT NULL DEFAULT 'pendente' CHECK (resultado IN ('pendente','criou','conciliou','duplicada','erro','ignorada')),
    recebida_em   TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Idempotencia exata: a mesma mensagem duas vezes nao gera trabalho.
    UNIQUE (origem_id, impressao)
);

CREATE UNIQUE INDEX ocorrencias_id_externo_idx ON ocorrencias (origem_id, id_externo)
    WHERE id_externo IS NOT NULL;

CREATE INDEX ocorrencias_lancamento_idx ON ocorrencias (lancamento_id);
