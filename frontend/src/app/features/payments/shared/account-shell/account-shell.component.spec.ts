import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { AccountApiService } from '../../../account/account-api.service';
import { AccountShellComponent } from './account-shell.component';

describe('AccountShellComponent', () => {
  let fixture: ComponentFixture<AccountShellComponent>;
  let mockAccountApi: jasmine.SpyObj<AccountApiService>;

  beforeEach(async () => {
    localStorage.clear();
    mockAccountApi = jasmine.createSpyObj<AccountApiService>('AccountApiService', ['me']);
    mockAccountApi.me.and.returnValue(
      of({
        id: 101,
        name: 'Nimali Perera',
        email: 'nimali@example.com',
        roles: ['buyer'],
      }),
    );

    await TestBed.configureTestingModule({
      imports: [AccountShellComponent],
      providers: [
        provideRouter([]),
        { provide: AccountApiService, useValue: mockAccountApi },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(AccountShellComponent);
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('renders the navy top bar and real user name, email, and initials from profile in the sidebar', () => {
    fixture.detectChanges();

    const nameEl = fixture.nativeElement.querySelector(
      '[data-testid="account-user-name"]',
    ) as HTMLElement;
    const emailEl = fixture.nativeElement.querySelector(
      '[data-testid="account-user-email"]',
    ) as HTMLElement;

    expect(nameEl?.textContent?.trim()).toBe('Nimali Perera');
    expect(nameEl?.textContent).not.toContain('Ruwan Fernando');
    expect(emailEl?.textContent?.trim()).toBe('nimali@example.com');
    expect(fixture.componentInstance.initials()).toBe('NP');
    expect(fixture.nativeElement.textContent).toContain('Global Handmade Marketplace');
  });

  it('falls back to localStorage user_name and hides email when profile lookup fails', () => {
    localStorage.setItem('user_name', 'Saman Kumara');
    mockAccountApi.me.and.returnValue(throwError(() => new Error('network error')));

    const fallbackFixture = TestBed.createComponent(AccountShellComponent);
    fallbackFixture.detectChanges();

    const nameEl = fallbackFixture.nativeElement.querySelector(
      '[data-testid="account-user-name"]',
    ) as HTMLElement;
    const emailEl = fallbackFixture.nativeElement.querySelector(
      '[data-testid="account-user-email"]',
    );

    expect(nameEl?.textContent?.trim()).toBe('Saman Kumara');
    expect(emailEl).toBeNull();
    expect(fallbackFixture.componentInstance.initials()).toBe('SK');
  });

  it('renders all six account navigation items plus Sign Out and marks the activeItem with aria-current="page"', () => {
    fixture.componentRef.setInput('activeItem', 'payments');
    fixture.detectChanges();

    const navEls = Array.from(
      fixture.nativeElement.querySelectorAll('[data-testid="account-nav-item"]'),
    ) as HTMLAnchorElement[];
    expect(navEls.length).toBe(6);

    const labels = navEls.map((el) => el.textContent?.trim());
    expect(labels).toEqual([
      'Orders',
      'Payments',
      'Returns & Refunds',
      'Addresses',
      'Notifications',
      'Account Settings',
    ]);

    const activeEl = fixture.nativeElement.querySelector(
      '[data-testid="account-nav-item"][data-nav-id="payments"]',
    ) as HTMLAnchorElement;
    expect(activeEl.getAttribute('aria-current')).toBe('page');
    expect(activeEl.className).toContain('bg-terracotta-100');
    expect(fixture.nativeElement.textContent).toContain('Sign Out');
  });

  it('shows the U.S.-only shipping info card only when showShippingInfo is true', () => {
    fixture.componentRef.setInput('showShippingInfo', false);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[data-testid="shipping-info-card"]')).toBeNull();

    fixture.componentRef.setInput('showShippingInfo', true);
    fixture.detectChanges();
    const card = fixture.nativeElement.querySelector(
      '[data-testid="shipping-info-card"]',
    ) as HTMLElement;
    expect(card).toBeTruthy();
    expect(card.textContent).toContain('U.S.-only shipping');
  });
});
