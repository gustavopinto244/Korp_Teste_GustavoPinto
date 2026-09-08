import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { FormArray } from '@angular/forms';
import { NotaForm } from './nota-form';
import { ItemSugerido } from '../../core/models/nota-fiscal';

describe('NotaForm', () => {
  let component: NotaForm;
  let fixture: ComponentFixture<NotaForm>;

  // O FormArray e os métodos do componente são `protected`: acesso por
  // índice é o caminho suportado pelo TypeScript para exercitá-los no teste.
  const itens = () => component['itens'] as FormArray;
  const adicionarLinha = () => (component['adicionarLinha'] as () => void)();

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [NotaForm],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    }).compileComponents();

    fixture = TestBed.createComponent(NotaForm);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('começa com uma única linha de item', () => {
    expect(itens().length).toBe(1);
  });

  it('adiciona múltiplas linhas de item fora do contexto de injeção', () => {
    // Regressão NG0203: `criarLinhaItem()` usava `takeUntilDestroyed()` sem
    // DestroyRef explícito, o que lança quando chamado de um handler de
    // evento (fora do contexto de injeção do inicializador de campo).
    expect(() => {
      adicionarLinha();
      adicionarLinha();
    }).not.toThrow();

    expect(itens().length).toBe(3);
    expect(component['sugestoesPorLinha']().length).toBe(3);
  });

  it('remove uma linha e mantém as sugestões alinhadas ao FormArray', () => {
    adicionarLinha();
    (component['removerLinha'] as (i: number) => void)(0);

    expect(itens().length).toBe(1);
    expect(component['sugestoesPorLinha']().length).toBe(1);
  });

  it('preenche o FormArray com as sugestões da IA, substituindo a linha vazia', () => {
    const sugestoes: ItemSugerido[] = [
      {
        produtoCodigo: 'PARAF-001',
        produtoDescricao: 'Parafuso sextavado M6',
        quantidade: 3,
        confianca: 'alta',
      },
      {
        produtoCodigo: 'MART-002',
        produtoDescricao: 'Martelo de borracha',
        quantidade: 2,
        confianca: 'alta',
      },
    ];

    (component['aoReceberSugestoes'] as (i: ItemSugerido[]) => void)(sugestoes);

    expect(itens().length).toBe(2);
    expect(itens().getRawValue()).toEqual([
      {
        produtoCodigo: 'PARAF-001',
        produtoDescricao: 'PARAF-001 - Parafuso sextavado M6',
        quantidade: 3,
      },
      {
        produtoCodigo: 'MART-002',
        produtoDescricao: 'MART-002 - Martelo de borracha',
        quantidade: 2,
      },
    ]);
    expect(component['sugestoesPorLinha']().length).toBe(2);
  });

  it('ignora uma interpretação da IA sem itens', () => {
    (component['aoReceberSugestoes'] as (i: ItemSugerido[]) => void)([]);

    expect(itens().length).toBe(1);
    expect(itens().at(0).value.produtoCodigo).toBe('');
  });
});
