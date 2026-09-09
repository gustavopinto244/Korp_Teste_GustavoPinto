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
  /**
   * O saldo que o formulário carregou ao abrir. O backend recusa a
   * atualização com 409 SALDO_DESATUALIZADO se o saldo já tiver mudado —
   * tipicamente porque uma nota foi impressa nesse meio-tempo. Sem isso, o
   * formulário salvaria o valor antigo por cima e desfaria o débito de uma
   * nota já fechada.
   */
  saldoEsperado?: number;
}
