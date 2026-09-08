import { Component, OnInit, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { MatChipsModule } from '@angular/material/chips';
import { MatTableModule } from '@angular/material/table';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBar } from '@angular/material/snack-bar';
import { NotaFiscal } from '../../core/models/nota-fiscal';
import { NotaFiscalService } from '../../core/services/nota-fiscal.service';
import { ErroApi } from '../../core/models/erro-api';
import { BotaoImprimir } from './botao-imprimir/botao-imprimir';

@Component({
  imports: [DatePipe, MatChipsModule, MatTableModule, MatProgressSpinnerModule, BotaoImprimir],
  selector: 'app-nota-detalhe',
  styleUrl: './nota-detalhe.scss',
  templateUrl: './nota-detalhe.html',
})
export class NotaDetalhe implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly notaFiscalService = inject(NotaFiscalService);
  private readonly snackBar = inject(MatSnackBar);

  protected readonly nota = signal<NotaFiscal | null>(null);
  protected readonly carregando = signal(true);
  protected readonly colunas = ['produtoCodigo', 'produtoDescricao', 'quantidade'];

  // Carrega a nota a partir do id na rota assim que o componente é montado.
  ngOnInit(): void {
    const id = Number(this.route.snapshot.paramMap.get('id'));
    this.notaFiscalService.obter(id).subscribe({
      next: (nota) => {
        this.nota.set(nota);
        this.carregando.set(false);
      },
      error: (erro: ErroApi) => {
        this.carregando.set(false);
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }

  // O botao-imprimir emite a nota já atualizada (Fechada) após sucesso;
  // basta refletir localmente sem nova requisição.
  protected aoAtualizarNota(nota: NotaFiscal): void {
    this.nota.set(nota);
  }
}
