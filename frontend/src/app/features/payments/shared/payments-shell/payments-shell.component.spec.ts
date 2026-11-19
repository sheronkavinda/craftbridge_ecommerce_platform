import { ComponentFixture, TestBed } from '@angular/core/testing';
import { PaymentsShellComponent } from './payments-shell.component';

describe('PaymentsShellComponent', () => {
  let fixture: ComponentFixture<PaymentsShellComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PaymentsShellComponent],
    }).compileComponents();

    fixture = TestBed.createComponent(PaymentsShellComponent);
    fixture.detectChanges();
  });

  it('renders the CraftBridge header wordmark, tagline, and category navigation', () => {
    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('CraftBridge');
    expect(text).toContain('SRI LANKA TO THE WORLD');
    expect(text).toContain('Handmade Crafts');
    expect(text).toContain('Meet the Artisans');
    expect(text).toContain('Ship to');
    expect(text).toContain('United States');
  });

  it('renders the four-column footer trust bar', () => {
    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('Authentic Sri Lankan Handmade');
    expect(text).toContain('U.S. Only Shipping');
    expect(text).toContain('Secure Payments');
    expect(text).toContain('Powered by PayHere');
    expect(text).toContain('Support Artisan Communities');
  });
});
