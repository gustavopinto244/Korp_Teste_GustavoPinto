-- Sem qualificação de schema: o runner de migrations posiciona o search_path
-- no schema do serviço (vindo do DSN) antes de executar este arquivo. É o que
-- permite a suíte de testes criar tudo num schema descartável em vez de no
-- schema de produção.

CREATE SEQUENCE nota_fiscal_numero_seq START WITH 1 INCREMENT BY 1;

CREATE TABLE nota_fiscal (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    numero     BIGINT NOT NULL DEFAULT nextval('nota_fiscal_numero_seq'),
    status     VARCHAR(10) NOT NULL DEFAULT 'Aberta',
    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
    fechado_em TIMESTAMPTZ NULL,

    CONSTRAINT uq_nota_numero UNIQUE (numero),
    CONSTRAINT ck_nota_status CHECK (status IN ('Aberta', 'Fechada'))
);

ALTER SEQUENCE nota_fiscal_numero_seq OWNED BY nota_fiscal.numero;

CREATE TABLE nota_fiscal_item (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nota_id           BIGINT NOT NULL REFERENCES nota_fiscal(id) ON DELETE CASCADE,
    produto_codigo    VARCHAR(50)  NOT NULL,
    produto_descricao VARCHAR(200) NOT NULL,
    quantidade        INTEGER NOT NULL,

    CONSTRAINT ck_item_quantidade_positiva CHECK (quantidade > 0)
);

CREATE INDEX idx_item_nota_id ON nota_fiscal_item (nota_id);
CREATE INDEX idx_nota_status ON nota_fiscal (status);
