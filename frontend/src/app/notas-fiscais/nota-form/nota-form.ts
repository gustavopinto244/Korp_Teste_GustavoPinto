import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
  FormArray,
  FormBuilder,
  ReactiveFormsModule,
  Validators,
} from '@angular/forms';
import { Router } from '@angular/router';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatAutocompleteModule } from '@angular/material/autocomplete';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBar } from '@angular/material/snack-bar';
import { AbstractControl } from '@angular/forms';
import { debounceTime, switchMap, of, catchError, Subject, takeUntil } from 'rxjs';
import { ProdutoService } from '../../core/services/produto.service';
import { NotaFiscalService } from '../../core/services/nota-fiscal.service';
import { Produto } from '../../core/models/produto';
import { ItemNotaCriacao, ItemSugerido } from '../../core/models/nota-fiscal';
import { ErroApi } from '../../core/models/erro-api';
import { IaSugestao } from './ia-sugestao/ia-sugestao';

@Component({
  imports: [
    ReactiveFormsModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatIconModule,
    MatAutocompleteModule,
    MatTooltipModule,
    MatProgressSpinnerModule,
    IaSugestao,
  ],
  selector: 'app-nota-form',
  styleUrl: './nota-form.scss',
  templateUrl: './nota-form.html',
})
export class NotaForm implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly produtoService = inject(ProdutoService);
  private readonly notaFiscalService = inject(NotaFiscalService);
  private readonly router = inject(Router);
  private readonly snackBar = inject(MatSnackBar);
  // Capturado uma única vez no contexto de injeção da classe: `criarLinhaItem()`
  // também é chamado de handlers de evento (adicionar item, sugestões da IA),
  // onde `takeUntilDestroyed()` sem argumento lançaria NG0203.
  private readonly destroyRef = inject(DestroyRef);

  protected readonly salvando = signal(false);
  protected readonly sugestoesPorLinha = signal<Produto[][]>([]);

  // Uma inscrição de vida longa por linha (o valueChanges do autocomplete)
  // não morre sozinha ao remover a linha do FormArray — só quando o
  // componente inteiro é destruído (takeUntilDestroyed). Este mapa guarda o
  // gatilho de encerramento por linha para fechar a inscrição órfã assim
  // que a linha é removida, sem esperar o componente inteiro morrer.
  private readonly encerramentoPorLinha = new Map<AbstractControl, Subject<void>>();

  protected readonly form = this.fb.group({
    itens: this.fb.array([this.criarLinhaItem()]),
  });

  protected get itens(): FormArray {
    return this.form.get('itens') as FormArray;
  }

  private criarLinhaItem() {
    const linha = this.fb.nonNullable.group({
      produtoCodigo: ['', Validators.required],
      produtoDescricao: [''],
      quantidade: [1, [Validators.required, Validators.min(1)]],
    });

    // Sinal de encerramento próprio desta linha: disparado em removerLinha().
    const encerrarLinha$ = new Subject<void>();
    this.encerramentoPorLinha.set(linha, encerrarLinha$);

    // Autocomplete de produto: busca a cada digitação, com debounce. A
    // chamada a `produtoService.listar()` bate no cache (`shareReplay(1)`)
    // depois da primeira vez, então o filtro abaixo já opera em memória.
    linha.controls.produtoDescricao.valueChanges
      .pipe(
        debounceTime(300),
        switchMap((termo) =>
          this.produtoService.listar().pipe(
            catchError(() => of<Produto[]>([])),
            switchMap((produtos) => {
              const filtro = (termo ?? '').toString().toLowerCase();
              return of(
                produtos.filter(
                  (p) =>
                    p.codigo.toLowerCase().includes(filtro) ||
                    p.descricao.toLowerCase().includes(filtro),
                ),
              );
            }),
          ),
        ),
        // Encerra quando a linha é removida (vida curta, ligada à linha) ou
        // quando o componente inteiro é destruído (vida longa, fallback).
        takeUntil(encerrarLinha$),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((produtos) => {
        const index = this.itens.controls.indexOf(linha);
        // Defesa adicional: mesmo com o encerramento explícito acima, uma
        // resposta que já estava em voo quando a linha foi removida não
        // deve virar uma propriedade solta no array de sugestões.
        if (index === -1) {
          return;
        }
        const atuais = [...this.sugestoesPorLinha()];
        atuais[index] = produtos;
        this.sugestoesPorLinha.set(atuais);
      });

    return linha;
  }

  // Nenhum estado externo para carregar: nota-form sempre inicia em modo de
  // criação com uma linha vazia; o array de sugestões acompanha o FormArray.
  ngOnInit(): void {
    this.sugestoesPorLinha.set([[]]);
  }

  protected selecionarProduto(index: number, produto: Produto): void {
    const linha = this.itens.at(index);
    linha.patchValue({
      produtoCodigo: produto.codigo,
      produtoDescricao: `${produto.codigo} - ${produto.descricao}`,
    });
  }

  protected adicionarLinha(): void {
    this.itens.push(this.criarLinhaItem());
    this.sugestoesPorLinha.set([...this.sugestoesPorLinha(), []]);
  }

  protected removerLinha(index: number): void {
    const linha = this.itens.at(index);
    this.encerramentoPorLinha.get(linha)?.next();
    this.encerramentoPorLinha.get(linha)?.complete();
    this.encerramentoPorLinha.delete(linha);

    this.itens.removeAt(index);
    const atuais = [...this.sugestoesPorLinha()];
    atuais.splice(index, 1);
    this.sugestoesPorLinha.set(atuais);
  }

  // Recebe as sugestões da IA e preenche o FormArray, substituindo a
  // primeira linha vazia (se houver) e adicionando as demais.
  protected aoReceberSugestoes(itensSugeridos: ItemSugerido[]): void {
    if (itensSugeridos.length === 0) {
      return;
    }

    const primeiraLinhaVazia =
      this.itens.length === 1 && !this.itens.at(0).value.produtoCodigo;

    if (primeiraLinhaVazia) {
      this.itens.removeAt(0);
      this.sugestoesPorLinha.set([]);
    }

    for (const item of itensSugeridos) {
      const linha = this.criarLinhaItem();
      linha.patchValue({
        produtoCodigo: item.produtoCodigo,
        produtoDescricao: `${item.produtoCodigo} - ${item.produtoDescricao}`,
        quantidade: item.quantidade,
      });
      this.itens.push(linha);
      this.sugestoesPorLinha.set([...this.sugestoesPorLinha(), []]);
    }
  }

  protected salvar(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }

    this.salvando.set(true);
    const itens: ItemNotaCriacao[] = this.itens
      .getRawValue()
      .map((item: { produtoCodigo: string; quantidade: number }) => ({
        produtoCodigo: item.produtoCodigo,
        quantidade: item.quantidade,
      }));

    this.notaFiscalService.criar({ itens }).subscribe({
      next: (nota) => {
        this.salvando.set(false);
        this.router.navigate(['/notas', nota.id]);
      },
      error: (erro: ErroApi) => {
        this.salvando.set(false);
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }

  protected cancelar(): void {
    this.router.navigate(['/notas']);
  }
}
