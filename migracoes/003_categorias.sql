-- 003: categorias fixas, regras de categorizacao e o vinculo no lancamento.
-- Regras decidem sozinhas (origem 'semente'/'aprendida') ou registram escolha
-- humana ('manual'); a precedencia e resolvida no dominio, aqui so os dados.

CREATE TABLE categorias (
    id   SMALLINT PRIMARY KEY,
    nome TEXT     NOT NULL UNIQUE
);

INSERT INTO categorias (id, nome) VALUES
    (1,  'mercado'),
    (2,  'restaurante'),
    (3,  'transporte'),
    (4,  'moradia'),
    (5,  'contas'),
    (6,  'saude'),
    (7,  'lazer'),
    (8,  'assinatura'),
    (9,  'educacao'),
    (10, 'vestuario'),
    (11, 'viagem'),
    (12, 'renda'),
    (13, 'transferencia'),
    (14, 'outros');

CREATE TABLE regras_categorizacao (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    categoria_id SMALLINT NOT NULL REFERENCES categorias (id),
    tipo         TEXT     NOT NULL CHECK (tipo IN ('exata','prefixo','contem','regex')),
    padrao       TEXT     NOT NULL CHECK (length(padrao) > 0),
    prioridade   SMALLINT NOT NULL DEFAULT 0,
    origem       TEXT     NOT NULL DEFAULT 'semente' CHECK (origem IN ('semente','aprendida','manual')),
    ativa        BOOLEAN  NOT NULL DEFAULT true,
    acertos      INTEGER  NOT NULL DEFAULT 0,
    erros        INTEGER  NOT NULL DEFAULT 0,
    criada_em    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Duas regras identicas apontando para categorias diferentes seria empate
    -- por dados; a unicidade transforma isso em erro de insercao.
    UNIQUE (tipo, padrao)
);

ALTER TABLE lancamentos
    ADD CONSTRAINT lancamentos_categoria_fk
    FOREIGN KEY (categoria_id) REFERENCES categorias (id);

-- Sementes conservadoras: so contraparte inequivoca. Padrao curto que pode
-- aparecer dentro de outra palavra entra como 'exata' (EXTRA dentro de
-- EXTRATO), nunca 'contem'. Digitos nao existem em contraparte_norm.
INSERT INTO regras_categorizacao (categoria_id, tipo, padrao) VALUES
    (1, 'contem',  'SUPERMERCADO'),
    (1, 'contem',  'CARREFOUR'),
    (1, 'contem',  'PAO DE ACUCAR'),
    (1, 'contem',  'ATACADAO'),
    (1, 'contem',  'ASSAI'),
    (1, 'exata',   'EXTRA'),
    (2, 'contem',  'IFOOD'),
    (2, 'contem',  'RAPPI'),
    (2, 'contem',  'RESTAURANTE'),
    (2, 'contem',  'PIZZARIA'),
    (2, 'contem',  'LANCHONETE'),
    (2, 'contem',  'PADARIA'),
    (3, 'contem',  'UBER'),
    (3, 'contem',  'POSTO'),
    (3, 'contem',  'ESTACIONAMENTO'),
    (3, 'contem',  'SEM PARAR'),
    (4, 'contem',  'CONDOMINIO'),
    (4, 'contem',  'ALUGUEL'),
    (4, 'contem',  'IMOBILIARIA'),
    (5, 'contem',  'SABESP'),
    (5, 'contem',  'ENEL'),
    (5, 'contem',  'COMGAS'),
    (6, 'contem',  'DROGARIA'),
    (6, 'contem',  'DROGASIL'),
    (6, 'contem',  'FARMACIA'),
    (6, 'contem',  'LABORATORIO'),
    (8, 'contem',  'NETFLIX'),
    (8, 'contem',  'SPOTIFY'),
    (8, 'contem',  'DISNEY'),
    (8, 'contem',  'GLOBOPLAY'),
    (8, 'prefixo', 'AMAZON PRIME');
