-- 001: o fato. Um lancamento e um movimento de dinheiro que aconteceu.
-- Nunca editar depois de aplicada; correcao e migracao nova.

CREATE TABLE lancamentos (
    id                 UUID        PRIMARY KEY,           -- UUIDv7, gerado em Go
    ocorrido_em        TIMESTAMPTZ NOT NULL,              -- instante do gasto, guardado em UTC
    competencia        DATE        NOT NULL,              -- 1o dia do mes, calculado em America/Sao_Paulo
    competencia_fatura DATE,                              -- so credito: mes da fatura
    valor_centavos     BIGINT      NOT NULL CHECK (valor_centavos <> 0),  -- negativo = saida
    meio               TEXT        NOT NULL CHECK (meio IN ('pix','credito','debito','boleto','dinheiro','transferencia')),
    contraparte        TEXT        NOT NULL,
    contraparte_norm   TEXT        NOT NULL,              -- maiuscula, sem acento, sem digito
    categoria_id       SMALLINT,                          -- FK entra na migracao de categorias
    categoria_origem   TEXT        NOT NULL DEFAULT 'pendente' CHECK (categoria_origem IN ('pendente','regra','manual','importacao')),
    situacao           TEXT        NOT NULL DEFAULT 'confirmado' CHECK (situacao IN ('provisorio','confirmado','descartado')),
    criado_em          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (competencia = date_trunc('month', competencia)::date)
);

CREATE INDEX lancamentos_competencia_idx ON lancamentos (competencia, ocorrido_em);

-- A fila de "perguntar categoria" e uma varredura minima: so o que falta.
CREATE INDEX lancamentos_sem_categoria_idx ON lancamentos (ocorrido_em)
    WHERE categoria_id IS NULL AND situacao <> 'descartado';
