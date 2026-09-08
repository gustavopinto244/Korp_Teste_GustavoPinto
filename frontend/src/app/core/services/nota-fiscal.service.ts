import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import {
  InterpretacaoResultado,
  NotaFiscal,
  NotaFiscalCriacao,
} from '../models/nota-fiscal';

// Caminho relativo: o nginx (produção) e o proxy do `ng serve`
// (desenvolvimento) repassam /api/faturamento para o serviço de faturamento.
// Assim a SPA nunca faz requisição cross-origin e não há host algum no bundle.
const BASE_URL = '/api/faturamento';

/**
 * Cliente HTTP fino para o serviço de faturamento. Sem lógica de UI: cada
 * método mapeia 1:1 para um endpoint do contrato.
 */
@Injectable({ providedIn: 'root' })
export class NotaFiscalService {
  private readonly http = inject(HttpClient);

  listar(): Observable<NotaFiscal[]> {
    return this.http.get<NotaFiscal[]>(`${BASE_URL}/notas`);
  }

  obter(id: number): Observable<NotaFiscal> {
    return this.http.get<NotaFiscal>(`${BASE_URL}/notas/${id}`);
  }

  criar(nota: NotaFiscalCriacao): Observable<NotaFiscal> {
    return this.http.post<NotaFiscal>(`${BASE_URL}/notas`, nota);
  }

  imprimir(id: number): Observable<NotaFiscal> {
    return this.http.post<NotaFiscal>(`${BASE_URL}/notas/${id}/imprimir`, {});
  }

  interpretar(texto: string): Observable<InterpretacaoResultado> {
    return this.http.post<InterpretacaoResultado>(`${BASE_URL}/notas/interpretar`, {
      texto,
    });
  }
}
