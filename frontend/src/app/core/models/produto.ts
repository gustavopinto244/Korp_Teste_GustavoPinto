export interface Produto {
  id: number;
  codigo: string;
  descricao: string;
  saldo: number;
}

export interface ProdutoCriacao {
  codigo: string;
  descricao: string;
  saldo: number;
}

export interface ProdutoAtualizacao {
  descricao: string;
  saldo: number;
}
