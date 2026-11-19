import { ComponentFixture, TestBed } from '@angular/core/testing';
import { PaymentStatus } from '../../payments.models';
import { PaymentStatusChipComponent } from './payment-status-chip.component';

describe('PaymentStatusChipComponent', () => {
  let fixture: ComponentFixture<PaymentStatusChipComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PaymentStatusChipComponent],
    }).compileComponents();

    fixture = TestBed.createComponent(PaymentStatusChipComponent);
  });

  it('renders the expected label and icon for all 5 PaymentStatus values', () => {
    const cases: Array<{ status: PaymentStatus; expectedLabel: string; expectedClass: string }> = [
      { status: 'PENDING', expectedLabel: 'Pending', expectedClass: 'bg-amber-100' },
      { status: 'SUCCESS', expectedLabel: 'Paid', expectedClass: 'bg-emerald-100' },
      { status: 'FAILED', expectedLabel: 'Failed', expectedClass: 'bg-red-100' },
      { status: 'CANCELLED', expectedLabel: 'Cancelled', expectedClass: 'bg-slate-200' },
      { status: 'EXPIRED', expectedLabel: 'Expired', expectedClass: 'bg-slate-200' },
    ];

    for (const tc of cases) {
      fixture.componentRef.setInput('status', tc.status);
      fixture.detectChanges();

      const chipEl = fixture.nativeElement.querySelector(
        '[data-testid="payment-status-chip"]',
      ) as HTMLElement;
      expect(chipEl).toBeTruthy();
      expect(chipEl.textContent?.trim()).toBe(tc.expectedLabel);
      expect(chipEl.className).toContain(tc.expectedClass);
      expect(chipEl.querySelector('svg')).toBeTruthy();
    }
  });
});
