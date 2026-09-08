import { Component, OnInit, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { MatTableModule } from '@angular/material/table';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatChipsModule } from '@angular/material/chips';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBar } from '@angular/material/snack-bar';
import { RouterLink } from '@angular/router';
import { NotaFiscal } from '../../core/models/nota-fiscal';
import { NotaFiscalService } from '../../core/services/nota-fiscal.service';
import { ErroApi } from '../../core/models/erro-api';

@Component({
  imports: [
    DatePipe,
    MatTableModule,
    MatButtonModule,
    MatIconModule,
    MatChipsModule,
    MatProgressSpinnerModule,
    RouterLink,
  ],
  selector: 'app-nota-lista',
  styleUrl: './nota-lista.scss',
  templateUrl: './nota-lista.html',
})
export class NotaLista implements OnInit {
  private readonly notaFiscalService = inject(NotaFiscalService);
  private readonly snackBar = inject(MatSnackBar);

  protected readonly notas = signal<NotaFiscal[]>([]);
  protected readonly carregando = signal(true);
  protected readonly colunas = ['numero', 'status', 'fechadoEm', 'acoes'];

  // Carrega a lista de notas assim que o componente é montado.
  ngOnInit(): void {
    this.carregando.set(true);
    this.notaFiscalService.listar().subscribe({
      next: (notas) => {
        this.notas.set(notas);
        this.carregando.set(false);
      },
      error: (erro: ErroApi) => {
        this.carregando.set(false);
        this.snackBar.open(erro.mensagem, 'Fechar', { duration: 5000 });
      },
    });
  }
}
