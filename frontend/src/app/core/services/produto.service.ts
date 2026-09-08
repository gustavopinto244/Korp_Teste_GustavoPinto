import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { Produto, ProdutoAtualizacao, ProdutoCriacao } from '../models/produto';

const BASE_URL = 'http://localhost:8081';

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
