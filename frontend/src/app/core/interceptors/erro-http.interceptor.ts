import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { catchError, throwError } from 'rxjs';
import { ErroApi } from '../models/erro-api';

/**
 * Traduz qualquer erro HTTP para o formato único `ErroApi` definido pelo
 * backend, garantindo que os componentes sempre recebam um objeto tipado e
 * pronto para exibição, mesmo quando a resposta não segue o contrato
 * (ex.: erro de rede, serviço totalmente fora do ar).
 */
export const erroHttpInterceptor: HttpInterceptorFn = (req, next) =>
  next(req).pipe(
    catchError((err: HttpErrorResponse) => {
      const erroApi = err.error?.erro as ErroApi | undefined;
      return throwError(
        () =>
          erroApi ??
          ({
            codigo: 'DESCONHECIDO',
            mensagem: 'Erro inesperado. Tente novamente.',
            tipo: 'sistema',
            repetivel: true,
          } satisfies ErroApi),
      );
    }),
  );
