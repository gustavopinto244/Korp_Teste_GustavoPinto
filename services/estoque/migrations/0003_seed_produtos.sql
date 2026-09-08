INSERT INTO produto (codigo, descricao, saldo) VALUES
    ('PARAF-001', 'Parafuso sextavado M6', 100),
    ('MART-002',  'Martelo de borracha', 30),
    ('DISCO-003', 'Disco de corte 115mm', 50),
    ('CHAVE-004', 'Chave de fenda 6mm', 40),
    ('LUVA-005',  'Luva de proteção couro', 25)
ON CONFLICT (codigo) DO NOTHING;
