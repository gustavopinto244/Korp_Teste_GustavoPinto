CREATE SCHEMA IF NOT EXISTS estoque;

CREATE TABLE estoque.produto (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    codigo        VARCHAR(50)  NOT NULL,
    descricao     VARCHAR(200) NOT NULL,
    saldo         INTEGER      NOT NULL DEFAULT 0,
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    atualizado_em TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_produto_codigo UNIQUE (codigo),
    CONSTRAINT ck_produto_saldo_nao_negativo CHECK (saldo >= 0)
);

CREATE INDEX idx_produto_codigo ON estoque.produto (codigo);
