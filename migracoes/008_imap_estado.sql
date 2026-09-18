-- 008: posicao de leitura da caixa IMAP. Linha unica: nao existe segunda
-- caixa de entrada, e a CHECK garante que ninguem inventa uma.

CREATE TABLE imap_estado (
    id          SMALLINT    PRIMARY KEY CHECK (id = 1),
    uidvalidity BIGINT      NOT NULL,
    ultima_uid  BIGINT      NOT NULL,
    atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
