import { Component, OnInit, inject, signal } from '@angular/core';
import { MatTableModule } from '@angular/material/table';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBar } from '@angular/material/snack-bar';
import { RouterLink } from '@angular/router';
import { Produto } from '../../core/models/produto';
import { ProdutoService } from '../../core/services/produto.service';
import { ErroApi } from '../../core/models/erro-api';

@Component({
  imports: [
    MatTableModule,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
  ],
  selector: 'app-produto-lista',
  styleUrl: './produto-lista.scss',
  templateUrl: './produto-lista.html',
})
export class ProdutoLista implements OnInit {
  private readonly produtoService = inject(ProdutoService);
  private readonly snackBar = inject(MatSnackBar);

  protected readonly produtos = signal<Produto[]>([]);
  protected readonly carregando = signal(true);
  protected readonly colunas = ['codigo', 'descricao', 'saldo', 'acoes'];

  // Carrega a lista de produtos assim que o componente é montado.
  ngOnInit(): void {
    this.carregar();
  }

  private carregar(): void {
    this.carregando.set(true);
    this.produtoService.listar().subscribe({
      next: (produtos) => {
        this.produtos.set(produtos);
        this.carregando.set(false);
      },
      error: (erro: ErroApi) => {
        this.carregando.set(false);
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }

  protected remover(produto: Produto): void {
    if (!confirm(`Remover o produto ${produto.codigo}?`)) {
      return;
    }
    this.produtoService.remover(produto.codigo).subscribe({
      next: () => this.carregar(),
      error: (erro: ErroApi) => {
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }
}
