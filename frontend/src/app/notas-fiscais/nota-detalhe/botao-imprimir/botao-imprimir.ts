import { Component, EventEmitter, Input, Output, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatSnackBar } from '@angular/material/snack-bar';
import { finalize } from 'rxjs';
import { NotaFiscalService } from '../../../core/services/nota-fiscal.service';
import { NotaFiscal } from '../../../core/models/nota-fiscal';
import { ErroApi } from '../../../core/models/erro-api';

type EstadoImpressao = 'idle' | 'processando' | 'sucesso' | 'erro';

/**
 * Botão de impressão de nota fiscal — a operação central do desafio.
 * Máquina de estados explícita (idle | processando | sucesso | erro) para
 * dar feedback claro ao usuário e proteger contra duplo clique, além da
 * proteção equivalente que já existe no backend (idempotência).
 */
@Component({
  imports: [MatButtonModule, MatProgressSpinnerModule, MatTooltipModule],
  selector: 'app-botao-imprimir',
  styleUrl: './botao-imprimir.scss',
  templateUrl: './botao-imprimir.html',
})
export class BotaoImprimir {
  private readonly notaFiscalService = inject(NotaFiscalService);
  private readonly snackBar = inject(MatSnackBar);

  @Input({ required: true }) nota!: NotaFiscal;
  @Output() notaAtualizada = new EventEmitter<NotaFiscal>();

  protected readonly estado = signal<EstadoImpressao>('idle');

  protected get desabilitado(): boolean {
    return this.nota.status !== 'Aberta' || this.estado() === 'processando';
  }

  protected get tooltip(): string | null {
    return this.nota.status !== 'Aberta' ? 'Esta nota já foi fechada' : null;
  }

  protected imprimir(): void {
    if (this.desabilitado) {
      return;
    }

    this.estado.set('processando');

    this.notaFiscalService
      .imprimir(this.nota.id)
      .pipe(
        // Garante que o estado nunca fique preso em "processando", mesmo se
        // a subscription for cancelada por qualquer motivo.
        finalize(() => {
          if (this.estado() === 'processando') {
            this.estado.set('idle');
          }
        }),
      )
      .subscribe({
        next: (nota) => {
          this.estado.set('sucesso');
          this.notaAtualizada.emit(nota);
          this.snackBar.open('Nota impressa com sucesso.', 'Fechar', {
            duration: 4000,
            panelClass: 'sucesso-impressao',
          });
        },
        error: (erro: ErroApi) => {
          this.estado.set('erro');
          this.snackBar.open(erro.mensagem, 'Fechar', {
            duration: 6000,
            panelClass: erro.tipo === 'negocio' ? 'erro-negocio' : 'erro-sistema',
          });
        },
      });
  }
}
