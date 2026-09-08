import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { ProdutoLista } from './produto-lista';

describe('ProdutoLista', () => {
  let component: ProdutoLista;
  let fixture: ComponentFixture<ProdutoLista>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [ProdutoLista],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    }).compileComponents();

    fixture = TestBed.createComponent(ProdutoLista);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
