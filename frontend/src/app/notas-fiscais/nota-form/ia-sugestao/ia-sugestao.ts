import { Component, EventEmitter, Output, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatIconModule } from '@angular/material/icon';
import { finalize } from 'rxjs';
import { NotaFiscalService } from '../../../core/services/nota-fiscal.service';
import { ItemSugerido, ItemNaoReconhecido } from '../../../core/models/nota-fiscal';

/**
 * Campo de texto livre que pede ao backend para interpretar uma descrição
 * em linguagem natural e sugerir itens de nota fiscal. Nunca envia nada
 * diretamente para a criação da nota — apenas emite sugestões para o
 * componente pai decidir o que fazer com elas.
 */
@Component({
  imports: [
    FormsModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatProgressSpinnerModule,
    MatIconModule,
  ],
  selector: 'app-ia-sugestao',
  styleUrl: './ia-sugestao.scss',
  templateUrl: './ia-sugestao.html',
})
export class IaSugestao {
  private readonly notaFiscalService = inject(NotaFiscalService);

  @Output() itensSugeridos = new EventEmitter<ItemSugerido[]>();

  protected texto = '';
  protected readonly interpretando = signal(false);
  protected readonly itensNaoReconhecidos = signal<ItemNaoReconhecido[]>([]);
  protected readonly mensagemErro = signal<string | null>(null);

  protected sugerir(): void {
    if (!this.texto.trim()) {
      return;
    }

    this.interpretando.set(true);
    this.mensagemErro.set(null);
    this.itensNaoReconhecidos.set([]);

    this.notaFiscalService
      .interpretar(this.texto)
      .pipe(finalize(() => this.interpretando.set(false)))
      .subscribe({
        next: (resultado) => {
          this.itensSugeridos.emit(resultado.itensSugeridos);
          this.itensNaoReconhecidos.set(resultado.itensNaoReconhecidos);
        },
        error: () => {
          this.mensagemErro.set(
            'Não foi possível interpretar agora, preencha manualmente.',
          );
        },
      });
  }
}
