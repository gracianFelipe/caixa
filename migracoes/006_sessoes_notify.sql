-- 006: sessoes do PWA e a ponte de tempo real (LISTEN/NOTIFY).

CREATE TABLE sessoes (
    -- sha256 hex do token que vive no cookie: vazamento do banco nao entrega
    -- sessao viva, e o token em si nunca e gravado.
    id         TEXT        PRIMARY KEY,
    criada_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ultimo_uso TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_em  TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessoes_expira_idx ON sessoes (expira_em);

-- Todo evento do outbox acorda quem estiver ouvindo: a API retransmite aos
-- clientes WebSocket. Cross-processo de graca — importar pelo caixactl ou
-- responder no Telegram (worker) tambem atualiza a tela aberta.
CREATE OR REPLACE FUNCTION notificar_evento() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('caixa_eventos', NEW.tipo);
    RETURN NEW;
END;
$$;

CREATE TRIGGER eventos_notificar
    AFTER INSERT ON eventos
    FOR EACH ROW EXECUTE FUNCTION notificar_evento();
