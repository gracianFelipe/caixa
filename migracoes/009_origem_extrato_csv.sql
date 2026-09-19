-- 009: o Bradesco do autor nao exporta OFX, so CSV. A origem e propria (nao
-- reaproveita extrato_ofx) porque a qualidade da evidencia difere: o OFX traz
-- FITID do banco, o CSV tem identificador sintetizado por nos. A conciliacao
-- de nivel 2 pontua por origem e nao pode confundir as duas.

INSERT INTO origens (id, nome) VALUES (5, 'extrato_csv');
