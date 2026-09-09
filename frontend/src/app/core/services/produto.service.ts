import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, shareReplay, tap } from 'rxjs';
import { Produto, ProdutoAtualizacao, ProdutoCriacao } from '../models/produto';

// Caminho relativo: o nginx (produção) e o proxy do `ng serve`
// (desenvolvimento) repassam /api/estoque para o serviço de estoque. Assim a
// SPA nunca faz requisição cross-origin e não há host algum no bundle.
const BASE_URL = '/api/estoque';

/**
 * Cliente HTTP fino para o serviço de estoque. Sem lógica de UI: cada
 * método mapeia 1:1 para um endpoint do contrato — com uma exceção
 * deliberada: `listar()` cacheia o catálogo.
 */
@Injectable({ providedIn: 'root' })
export class ProdutoService {
  private readonly http = inject(HttpClient);

  // O autocomplete de produto em nota-form.ts chama `listar()` a cada
  // digitação (com debounce). Sem cache isso baixava o catálogo inteiro por
  // tecla. `shareReplay(1)` faz a primeira chamada disparar o HTTP e todas
  // as seguintes reaproveitarem o mesmo resultado em memória, até o cache
  // ser invalidado por uma escrita.
  private cacheDeProdutos$: Observable<Produto[]> | null = null;

  listar(): Observable<Produto[]> {
    this.cacheDeProdutos$ ??= this.http
      .get<Produto[]>(`${BASE_URL}/produtos`)
      .pipe(shareReplay(1));
    return this.cacheDeProdutos$;
  }

  obter(codigo: string): Observable<Produto> {
    return this.http.get<Produto>(`${BASE_URL}/produtos/${codigo}`);
  }

  criar(produto: ProdutoCriacao): Observable<Produto> {
    return this.http
      .post<Produto>(`${BASE_URL}/produtos`, produto)
      .pipe(tap(() => this.invalidarCache()));
  }

  atualizar(codigo: string, produto: ProdutoAtualizacao): Observable<Produto> {
    return this.http
      .put<Produto>(`${BASE_URL}/produtos/${codigo}`, produto)
      .pipe(tap(() => this.invalidarCache()));
  }

  remover(codigo: string): Observable<void> {
    return this.http
      .delete<void>(`${BASE_URL}/produtos/${codigo}`)
      .pipe(tap(() => this.invalidarCache()));
  }

  // Chamado após qualquer escrita (criar/atualizar/remover): o próximo
  // `listar()` volta a bater no backend em vez de devolver dado obsoleto.
  private invalidarCache(): void {
    this.cacheDeProdutos$ = null;
  }
}
