CREATE TABLE idempotencia_baixa (
    chave         VARCHAR(100) PRIMARY KEY,
    status_http   INTEGER      NOT NULL,
    resposta_json JSONB        NOT NULL,
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_idempotencia_criado_em ON idempotencia_baixa (criado_em);
