import { Component, OnInit, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBar } from '@angular/material/snack-bar';
import { ProdutoService } from '../../core/services/produto.service';
import { ErroApi } from '../../core/models/erro-api';

@Component({
  imports: [
    ReactiveFormsModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatProgressSpinnerModule,
  ],
  selector: 'app-produto-form',
  styleUrl: './produto-form.scss',
  templateUrl: './produto-form.html',
})
export class ProdutoForm implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly produtoService = inject(ProdutoService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly snackBar = inject(MatSnackBar);

  protected readonly salvando = signal(false);
  protected readonly carregando = signal(false);
  protected readonly modoEdicao = signal(false);
  // O saldo que veio do backend ao abrir o formulário, enviado de volta no
  // salvamento para o estoque detectar se ele mudou nesse meio-tempo.
  private readonly saldoCarregado = signal<number | null>(null);

  protected readonly form = this.fb.nonNullable.group({
    codigo: ['', Validators.required],
    descricao: ['', Validators.required],
    saldo: [0, [Validators.required, Validators.min(0)]],
  });

  // Em modo edição, carrega o produto existente a partir do código na rota.
  ngOnInit(): void {
    const codigo = this.route.snapshot.paramMap.get('codigo');
    if (!codigo) {
      return;
    }
    this.modoEdicao.set(true);
    this.form.controls.codigo.disable();
    this.carregando.set(true);
    this.produtoService.obter(codigo).subscribe({
      next: (produto) => {
        this.form.patchValue(produto);
        this.saldoCarregado.set(produto.saldo);
        this.carregando.set(false);
      },
      error: (erro: ErroApi) => {
        this.carregando.set(false);
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }

  protected salvar(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }

    this.salvando.set(true);
    const valor = this.form.getRawValue();

    const operacao = this.modoEdicao()
      ? this.produtoService.atualizar(valor.codigo, {
          descricao: valor.descricao,
          saldo: valor.saldo,
          saldoEsperado: this.saldoCarregado() ?? undefined,
        })
      : this.produtoService.criar(valor);

    operacao.subscribe({
      next: () => {
        this.salvando.set(false);
        this.router.navigate(['/produtos']);
      },
      error: (erro: ErroApi) => {
        this.salvando.set(false);
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }

  protected cancelar(): void {
    this.router.navigate(['/produtos']);
  }
}
