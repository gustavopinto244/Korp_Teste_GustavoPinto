import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { NotaLista } from './nota-lista';

describe('NotaLista', () => {
  let component: NotaLista;
  let fixture: ComponentFixture<NotaLista>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [NotaLista],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    }).compileComponents();

    fixture = TestBed.createComponent(NotaLista);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
