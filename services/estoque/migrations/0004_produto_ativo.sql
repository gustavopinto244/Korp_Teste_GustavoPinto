-- Exclusão de produto passa a ser lógica.
--
-- Antes, DELETE /produtos/{codigo} apagava a linha. Uma nota fiscal Aberta
-- que referenciasse aquele produto nunca mais podia ser impressa: a baixa
-- respondia PRODUTO_NAO_ENCONTRADO e a nota ficava Aberta para sempre, sem
-- caminho de saída — o status só vai de Aberta para Fechada.
--
-- Não há (nem pode haver) FK entre os schemas: o estoque não conhece nota
-- fiscal, e consultar o faturamento para saber se um produto está em uso
-- inverteria a direção da dependência entre os serviços. A coluna resolve
-- pelo lado do estoque: o produto some do catálogo mas continua existindo
-- para as notas que já o referenciam.
ALTER TABLE produto
    ADD COLUMN ativo BOOLEAN NOT NULL DEFAULT true;

-- O catálogo (GET /produtos) filtra por ativo; este índice parcial serve
-- exatamente essa consulta.
CREATE INDEX idx_produto_ativo ON produto (codigo) WHERE ativo;
