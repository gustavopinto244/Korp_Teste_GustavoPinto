import { Routes } from '@angular/router';

export const routes: Routes = [
  { path: '', redirectTo: 'produtos', pathMatch: 'full' },
  {
    path: 'produtos',
    loadComponent: () =>
      import('./produtos/produto-lista/produto-lista').then((m) => m.ProdutoLista),
  },
  {
    path: 'produtos/novo',
    loadComponent: () =>
      import('./produtos/produto-form/produto-form').then((m) => m.ProdutoForm),
  },
  {
    path: 'produtos/:codigo/editar',
    loadComponent: () =>
      import('./produtos/produto-form/produto-form').then((m) => m.ProdutoForm),
  },
  {
    path: 'notas',
    loadComponent: () =>
      import('./notas-fiscais/nota-lista/nota-lista').then((m) => m.NotaLista),
  },
  {
    path: 'notas/nova',
    loadComponent: () =>
      import('./notas-fiscais/nota-form/nota-form').then((m) => m.NotaForm),
  },
  {
    path: 'notas/:id',
    loadComponent: () =>
      import('./notas-fiscais/nota-detalhe/nota-detalhe').then((m) => m.NotaDetalhe),
  },
];
