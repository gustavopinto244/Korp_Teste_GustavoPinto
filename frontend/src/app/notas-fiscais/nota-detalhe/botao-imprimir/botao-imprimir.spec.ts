import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { BotaoImprimir } from './botao-imprimir';
import { NotaFiscal } from '../../../core/models/nota-fiscal';

describe('BotaoImprimir', () => {
  let component: BotaoImprimir;
  let fixture: ComponentFixture<BotaoImprimir>;

  const nota: NotaFiscal = { id: 1, numero: 1, status: 'Aberta', itens: [] };

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BotaoImprimir],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();

    fixture = TestBed.createComponent(BotaoImprimir);
    component = fixture.componentInstance;
    fixture.componentRef.setInput('nota', nota);
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should disable the button when nota is Fechada', async () => {
    fixture.componentRef.setInput('nota', { ...nota, status: 'Fechada' });
    await fixture.whenStable();
    const button: HTMLButtonElement = fixture.nativeElement.querySelector('button');
    expect(button.disabled).toBe(true);
  });
});
