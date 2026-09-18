-- 005: limites por categoria, alertas emitidos e a pergunta ganha tipo.

CREATE TABLE orcamentos (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    categoria_id    SMALLINT NOT NULL REFERENCES categorias (id),
    competencia     DATE,                -- NULL = limite padrao de todo mes
    limite_centavos BIGINT   NOT NULL CHECK (limite_centavos > 0),
    criado_em       TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Sem isto, dois limites-padrao da mesma categoria coexistiriam:
    -- UNIQUE comum trata NULL como sempre-distinto.
    UNIQUE NULLS NOT DISTINCT (categoria_id, competencia)
);

CREATE TABLE alertas (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tipo      TEXT NOT NULL,
    chave     TEXT NOT NULL,
    criado_em TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- O aviso de 80% nao vai 14 vezes: a constraint e o guardiao, nao logica.
    UNIQUE (tipo, chave)
);

ALTER TABLE perguntas
    ADD COLUMN tipo TEXT NOT NULL DEFAULT 'categoria'
        CHECK (tipo IN ('categoria', 'conciliacao')),
    ADD COLUMN referencia UUID REFERENCES lancamentos (id);
