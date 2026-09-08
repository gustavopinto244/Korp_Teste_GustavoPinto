export type StatusNota = 'Aberta' | 'Fechada';

export interface ItemNota {
  produtoCodigo: string;
  produtoDescricao: string;
  quantidade: number;
}

export interface NotaFiscal {
  id: number;
  numero: number;
  status: StatusNota;
  itens: ItemNota[];
  fechadoEm?: string;
}

export interface ItemNotaCriacao {
  produtoCodigo: string;
  quantidade: number;
}

export interface NotaFiscalCriacao {
  itens: ItemNotaCriacao[];
}

export type NivelConfianca = 'alta' | 'media' | 'baixa';

export interface ItemSugerido {
  produtoCodigo: string;
  produtoDescricao: string;
  quantidade: number;
  confianca: NivelConfianca;
}

export interface ItemNaoReconhecido {
  textoOriginal: string;
  motivo: string;
}

export interface InterpretacaoResultado {
  itensSugeridos: ItemSugerido[];
  itensNaoReconhecidos: ItemNaoReconhecido[];
}
