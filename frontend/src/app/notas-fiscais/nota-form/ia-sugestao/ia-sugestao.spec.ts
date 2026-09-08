import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { IaSugestao } from './ia-sugestao';

describe('IaSugestao', () => {
  let component: IaSugestao;
  let fixture: ComponentFixture<IaSugestao>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [IaSugestao],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();

    fixture = TestBed.createComponent(IaSugestao);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
