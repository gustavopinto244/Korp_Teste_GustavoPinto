import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { Produto, ProdutoAtualizacao, ProdutoCriacao } from '../models/produto';

// Caminho relativo: o nginx (produção) e o proxy do `ng serve`
// (desenvolvimento) repassam /api/estoque para o serviço de estoque. Assim a
// SPA nunca faz requisição cross-origin e não há host algum no bundle.
const BASE_URL = '/api/estoque';

/**
 * Cliente HTTP fino para o serviço de estoque. Sem lógica de UI: cada
 * método mapeia 1:1 para um endpoint do contrato.
 */
@Injectable({ providedIn: 'root' })
export class ProdutoService {
  private readonly http = inject(HttpClient);

  listar(): Observable<Produto[]> {
    return this.http.get<Produto[]>(`${BASE_URL}/produtos`);
  }

  obter(codigo: string): Observable<Produto> {
    return this.http.get<Produto>(`${BASE_URL}/produtos/${codigo}`);
  }

  criar(produto: ProdutoCriacao): Observable<Produto> {
    return this.http.post<Produto>(`${BASE_URL}/produtos`, produto);
  }

  atualizar(codigo: string, produto: ProdutoAtualizacao): Observable<Produto> {
    return this.http.put<Produto>(`${BASE_URL}/produtos/${codigo}`, produto);
  }

  remover(codigo: string): Observable<void> {
    return this.http.delete<void>(`${BASE_URL}/produtos/${codigo}`);
  }
}
