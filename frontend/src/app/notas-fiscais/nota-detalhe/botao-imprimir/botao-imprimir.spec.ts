import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { MatSnackBar } from '@angular/material/snack-bar';
import { BotaoImprimir } from './botao-imprimir';
import { NotaFiscal } from '../../../core/models/nota-fiscal';
import { ErroApi } from '../../../core/models/erro-api';
import { erroHttpInterceptor } from '../../../core/interceptors/erro-http.interceptor';

describe('BotaoImprimir', () => {
  let component: BotaoImprimir;
  let fixture: ComponentFixture<BotaoImprimir>;
  let http: HttpTestingController;
  let snackBarAberto: { mensagem: string; panelClass?: string | string[] } | null;

  const nota: NotaFiscal = { id: 1, numero: 1, status: 'Aberta', itens: [] };
  const URL_IMPRESSAO = '/api/faturamento/notas/1/imprimir';

  // `estado` e `imprimir` são `protected`: acesso por índice é o caminho
  // suportado pelo TypeScript para exercitá-los no teste.
  const estado = () => (component['estado'] as () => string)();
  const imprimir = () => (component['imprimir'] as () => void)();
  const botao = (): HTMLButtonElement => fixture.nativeElement.querySelector('button');

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BotaoImprimir],
      providers: [
        // Interceptor incluído de propósito: é ele que converte a resposta de
        // erro do backend no `ErroApi` que o componente exibe.
        provideHttpClient(withInterceptors([erroHttpInterceptor])),
        provideHttpClientTesting(),
      ],
    }).compileComponents();

    http = TestBed.inject(HttpTestingController);

    snackBarAberto = null;
    const snackBar = TestBed.inject(MatSnackBar);
    snackBar.open = ((mensagem: string, _acao?: string, config?: { panelClass?: string | string[] }) => {
      snackBarAberto = { mensagem, panelClass: config?.panelClass };
      return null as never;
    }) as typeof snackBar.open;

    fixture = TestBed.createComponent(BotaoImprimir);
    component = fixture.componentInstance;
    fixture.componentRef.setInput('nota', nota);
    await fixture.whenStable();
  });

  afterEach(() => {
    http.verify();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('desabilita o botão quando a nota está Fechada', async () => {
    fixture.componentRef.setInput('nota', { ...nota, status: 'Fechada' });
    await fixture.whenStable();

    expect(botao().disabled).toBe(true);
  });

  it('não dispara requisição quando a nota está Fechada', async () => {
    fixture.componentRef.setInput('nota', { ...nota, status: 'Fechada' });
    await fixture.whenStable();

    imprimir();

    http.expectNone(URL_IMPRESSAO);
    expect(estado()).toBe('idle');
  });

  it('entra em processando e desabilita o botão enquanto a requisição corre', async () => {
    imprimir();
    await fixture.whenStable();

    expect(estado()).toBe('processando');
    expect(botao().disabled).toBe(true);
    expect(fixture.nativeElement.textContent).toContain('Imprimindo');

    http.expectOne(URL_IMPRESSAO).flush({ ...nota, status: 'Fechada' });
  });

  it('vai para sucesso e emite a nota atualizada', async () => {
    let emitida: NotaFiscal | null = null;
    component.notaAtualizada.subscribe((n) => (emitida = n));

    imprimir();
    http.expectOne(URL_IMPRESSAO).flush({ ...nota, status: 'Fechada' });
    await fixture.whenStable();

    expect(estado()).toBe('sucesso');
    expect(emitida).toEqual({ ...nota, status: 'Fechada' });
    expect(snackBarAberto?.mensagem).toBe('Nota impressa com sucesso.');
  });

  it('exibe a mensagem do ErroApi e sai de processando quando a impressão falha', async () => {
    const erro: ErroApi = {
      codigo: 'SALDO_INSUFICIENTE',
      mensagem: 'Saldo insuficiente para o produto PARAF-001.',
      tipo: 'negocio',
      repetivel: false,
    };

    imprimir();
    expect(estado()).toBe('processando');

    http
      .expectOne(URL_IMPRESSAO)
      .flush({ erro }, { status: 422, statusText: 'Unprocessable Entity' });
    await fixture.whenStable();

    expect(estado()).toBe('erro');
    // O `finalize` do pipe garante que o estado nunca fique preso em
    // "processando" — e o botão volta a ficar clicável, pois a nota
    // permanece Aberta.
    expect(estado()).not.toBe('processando');
    expect(botao().disabled).toBe(false);
    expect(snackBarAberto?.mensagem).toBe(erro.mensagem);
    expect(snackBarAberto?.panelClass).toBe('erro-negocio');
  });

  it('sai de processando quando o serviço está totalmente fora do ar', async () => {
    imprimir();
    expect(estado()).toBe('processando');

    http.expectOne(URL_IMPRESSAO).error(new ProgressEvent('error'));
    await fixture.whenStable();

    expect(estado()).toBe('erro');
    expect(botao().disabled).toBe(false);
    expect(snackBarAberto?.mensagem).toBe('Erro inesperado. Tente novamente.');
    expect(snackBarAberto?.panelClass).toBe('erro-sistema');
  });
});
