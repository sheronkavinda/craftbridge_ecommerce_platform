import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { AccountApiService } from '../../../account/account-api.service';
import { SellerShellComponent } from './seller-shell.component';

describe('SellerShellComponent', () => {
  let fixture: ComponentFixture<SellerShellComponent>;
  let mockAccountApi: jasmine.SpyObj<AccountApiService>;

  beforeEach(async () => {
    localStorage.clear();
    mockAccountApi = jasmine.createSpyObj<AccountApiService>('AccountApiService', ['me']);
    mockAccountApi.me.and.returnValue(
      of({
        id: 202,
        name: 'Amaya Crafts',
        email: 'amaya@example.com',
        roles: ['seller'],
      }),
    );

    await TestBed.configureTestingModule({
      imports: [SellerShellComponent],
      providers: [
        provideRouter([]),
        { provide: AccountApiService, useValue: mockAccountApi },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(SellerShellComponent);
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('renders the seller navigation items with Earnings active and displays the real seller identity', () => {
    fixture.detectChanges();

    const navEls = Array.from(
      fixture.nativeElement.querySelectorAll('[data-testid="seller-nav-item"]'),
    ) as HTMLAnchorElement[];
    expect(navEls.length).toBe(8);

    const activeEl = fixture.nativeElement.querySelector(
      '[data-testid="seller-nav-item"][data-nav-id="earnings"]',
    ) as HTMLAnchorElement;
    expect(activeEl.getAttribute('aria-current')).toBe('page');
    expect(activeEl.className).toContain('bg-terracotta');

    const identityText = (
      fixture.nativeElement.querySelector('[data-testid="seller-identity"]') as HTMLElement
    ).textContent;
    expect(identityText).toContain('Amaya Crafts');
    expect(identityText).not.toContain('Kasun Pottery');
    expect(fixture.componentInstance.initials()).toBe('AC');
    expect(identityText).toContain('Seller · Sri Lanka');
  });

  it('falls back to localStorage user_name when profile lookup fails', () => {
    localStorage.setItem('user_name', 'Lanka Woodworks');
    mockAccountApi.me.and.returnValue(throwError(() => new Error('offline')));

    const fallbackFixture = TestBed.createComponent(SellerShellComponent);
    fallbackFixture.detectChanges();

    const nameEl = fallbackFixture.nativeElement.querySelector(
      '[data-testid="seller-name"]',
    ) as HTMLElement;
    expect(nameEl.textContent?.trim()).toBe('Lanka Woodworks');
    expect(fallbackFixture.componentInstance.initials()).toBe('LW');
  });

  it('toggles the mobile seller navigation menu when the menu button is clicked', () => {
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[data-testid="seller-mobile-nav"]')).toBeNull();

    const btn = fixture.nativeElement.querySelector(
      '[data-testid="seller-mobile-menu-btn"]',
    ) as HTMLButtonElement;
    btn.click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('[data-testid="seller-mobile-nav"]')).toBeTruthy();
  });
});
