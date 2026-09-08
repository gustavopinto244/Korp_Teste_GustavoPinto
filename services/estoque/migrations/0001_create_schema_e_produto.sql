-- Sem qualificação de schema: o runner de migrations (internal/migrate)
-- cria o schema alvo derivado do search_path da conexão e aplica cada
-- migration com SET LOCAL search_path nesse schema. Assim o mesmo arquivo
-- serve ao schema de produção ("estoque") e ao schema de teste
-- ("estoque_test"), sem que a suíte toque nos dados reais.
CREATE TABLE produto (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    codigo        VARCHAR(50)  NOT NULL,
    descricao     VARCHAR(200) NOT NULL,
    saldo         INTEGER      NOT NULL DEFAULT 0,
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    atualizado_em TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_produto_codigo UNIQUE (codigo),
    CONSTRAINT ck_produto_saldo_nao_negativo CHECK (saldo >= 0)
);

CREATE INDEX idx_produto_codigo ON produto (codigo);
