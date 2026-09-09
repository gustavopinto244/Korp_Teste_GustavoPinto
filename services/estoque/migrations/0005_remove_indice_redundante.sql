-- idx_produto_codigo duplicava o índice que a constraint UNIQUE (codigo) já
-- cria: dois btree idênticos sobre a mesma coluna, ambos mantidos a cada
-- INSERT e UPDATE, sem que nenhuma consulta pudesse aproveitar o segundo.
DROP INDEX IF EXISTS idx_produto_codigo;
