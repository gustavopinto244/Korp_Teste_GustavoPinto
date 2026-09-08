export interface ErroApi {
  codigo: string;
  mensagem: string;
  tipo: 'negocio' | 'sistema';
  repetivel: boolean;
}
