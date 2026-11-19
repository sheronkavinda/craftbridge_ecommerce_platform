import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { AccountShellComponent } from './account-shell.component';

describe('AccountShellComponent', () => {
  let fixture: ComponentFixture<AccountShellComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AccountShellComponent],
      providers: [provideRouter([])],
    }).compileComponents();

    fixture = TestBed.createComponent(AccountShellComponent);
  });

  it('renders the navy top bar and buyer actor name, email, and initials in the sidebar', () => {
    fixture.detectChanges();

    const nameEl = fixture.nativeElement.querySelector(
      '[data-testid="account-user-name"]',
    ) as HTMLElement;
    const emailEl = fixture.nativeElement.querySelector(
      '[data-testid="account-user-email"]',
    ) as HTMLElement;

    expect(nameEl?.textContent?.trim()).toBe('Ruwan Fernando');
    expect(emailEl?.textContent?.trim()).toBe('ruwan@craftbridge.test');
    expect(fixture.componentInstance.initials()).toBe('RF');
    expect(fixture.nativeElement.textContent).toContain('Global Handmade Marketplace');
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
