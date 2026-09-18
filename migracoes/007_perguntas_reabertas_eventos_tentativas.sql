-- 007: duas correcoes de modelo achadas em auditoria.

-- A UNIQUE total de perguntas impedia a SEGUNDA pergunta legitima do mesmo
-- lancamento: conciliacao respondida ("gasto novo") precisa abrir a pergunta
-- de categoria. A regra verdadeira e "uma pergunta ABERTA por lancamento".
ALTER TABLE perguntas DROP CONSTRAINT perguntas_lancamento_id_key;
CREATE UNIQUE INDEX perguntas_aberta_unica_idx
    ON perguntas (lancamento_id) WHERE estado = 'aberta';

-- Evento que falha de forma deterministica (dado corrompido, bug) nao pode
-- bloquear a fila para sempre (head-of-line): depois de 5 tentativas ele sai
-- da varredura e fica visivel para inspecao manual.
ALTER TABLE eventos ADD COLUMN tentativas SMALLINT NOT NULL DEFAULT 0;

DROP INDEX eventos_pendentes_idx;
CREATE INDEX eventos_pendentes_idx ON eventos (criado_em)
    WHERE processado_em IS NULL AND tentativas < 5;
