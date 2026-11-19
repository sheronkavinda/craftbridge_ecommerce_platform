import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { SellerShellComponent } from './seller-shell.component';

describe('SellerShellComponent', () => {
  let fixture: ComponentFixture<SellerShellComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [SellerShellComponent],
      providers: [provideRouter([])],
    }).compileComponents();

    fixture = TestBed.createComponent(SellerShellComponent);
  });

  it('renders the seller navigation items with Earnings active and displays the seller identity', () => {
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
    expect(identityText).toContain('Kasun Pottery');
    expect(identityText).toContain('Seller · Sri Lanka');
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
