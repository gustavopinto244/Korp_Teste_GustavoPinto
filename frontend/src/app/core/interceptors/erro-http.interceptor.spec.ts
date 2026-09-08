import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { ErroApi } from '../models/erro-api';
import { erroHttpInterceptor } from './erro-http.interceptor';

describe('erroHttpInterceptor', () => {
  let http: HttpClient;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(withInterceptors([erroHttpInterceptor])),
        provideHttpClientTesting(),
      ],
    });

    http = TestBed.inject(HttpClient);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpMock.verify();
  });

  it('converte o envelope { erro: ... } do backend em ErroApi tipado', async () => {
    const erro: ErroApi = {
      codigo: 'NOTA_JA_FECHADA',
      mensagem: 'Esta nota já foi fechada.',
      tipo: 'negocio',
      repetivel: false,
    };

    const recebido = new Promise<ErroApi>((resolve) => {
      http.get('/api/faturamento/notas/1').subscribe({
        error: (e: ErroApi) => resolve(e),
      });
    });

    httpMock
      .expectOne('/api/faturamento/notas/1')
      .flush({ erro }, { status: 409, statusText: 'Conflict' });

    await expect(recebido).resolves.toEqual(erro);
  });

  it('usa o fallback DESCONHECIDO quando a resposta não segue o contrato', async () => {
    const recebido = new Promise<ErroApi>((resolve) => {
      http.get('/api/estoque/produtos').subscribe({
        error: (e: ErroApi) => resolve(e),
      });
    });

    httpMock
      .expectOne('/api/estoque/produtos')
      .flush('<html>502 Bad Gateway</html>', {
        status: 502,
        statusText: 'Bad Gateway',
      });

    await expect(recebido).resolves.toEqual({
      codigo: 'DESCONHECIDO',
      mensagem: 'Erro inesperado. Tente novamente.',
      tipo: 'sistema',
      repetivel: true,
    } satisfies ErroApi);
  });

  it('usa o fallback DESCONHECIDO em falha de rede, sem corpo de resposta', async () => {
    const recebido = new Promise<ErroApi>((resolve) => {
      http.get('/api/estoque/produtos').subscribe({
        error: (e: ErroApi) => resolve(e),
      });
    });

    httpMock.expectOne('/api/estoque/produtos').error(new ProgressEvent('error'));

    const erro = await recebido;
    expect(erro.codigo).toBe('DESCONHECIDO');
    expect(erro.tipo).toBe('sistema');
    expect(erro.repetivel).toBe(true);
  });

  it('não interfere em respostas bem-sucedidas', async () => {
    const recebido = new Promise<unknown>((resolve) => {
      http.get('/api/estoque/produtos').subscribe({ next: resolve });
    });

    httpMock.expectOne('/api/estoque/produtos').flush([{ codigo: 'PARAF-001' }]);

    await expect(recebido).resolves.toEqual([{ codigo: 'PARAF-001' }]);
  });
});
