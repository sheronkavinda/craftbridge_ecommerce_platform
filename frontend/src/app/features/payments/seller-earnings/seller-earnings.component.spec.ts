import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { SellerEarningsResponse, SellerTransaction } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import {
  SellerEarningsComponent,
  deriveCommissionPercent,
} from './seller-earnings.component';

describe('SellerEarningsComponent', () => {
  let fixture: ComponentFixture<SellerEarningsComponent>;
  let mockPaymentsApi: jasmine.SpyObj<PaymentsApiService>;
  let router: Router;

  const sampleTransactions: SellerTransaction[] = [
    {
      order_id: 'order-1024',
      order_number: 'CB-1024',
      date: '2026-11-16T10:00:00Z',
      product_title: 'Handwoven Rattan Pendant Lamp',
      gross_cents: 6000,
      shipping_cents: 800,
      commission_cents: 600,
      refund_cents: 0,
      adjustments_cents: 0,
      net_cents: 6200,
      status: 'COMPLETED',
    },
    {
      order_id: 'order-1023',
      order_number: 'CB-1023',
      date: '2026-11-14T10:00:00Z',
      product_title: 'Ceramic Serving Bowl',
      gross_cents: 4500,
      shipping_cents: 700,
      commission_cents: 450,
      refund_cents: 5200,
      adjustments_cents: 0,
      net_cents: -450,
      status: 'REFUNDED',
    },
  ];

  const sampleResponse: SellerEarningsResponse = {
    data: {
      summary: {
        pending_cents: 34000,
        available_cents: 62000,
        paid_cents: 88000,
        currency: 'USD',
      },
      transactions: sampleTransactions,
      payouts: [],
    },
    meta: { page: 1, page_size: 20, total: 2 },
  };

  beforeEach(async () => {
    mockPaymentsApi = jasmine.createSpyObj<PaymentsApiService>('PaymentsApiService', [
      'getSellerEarnings',
    ]);

    await TestBed.configureTestingModule({
      imports: [SellerEarningsComponent],
      providers: [
        provideRouter([]),
        { provide: PaymentsApiService, useValue: mockPaymentsApi },
        {
          provide: ActivatedRoute,
          useValue: {
            snapshot: {
              queryParamMap: convertToParamMap({}),
            },
          },
        },
      ],
    }).compileComponents();

    router = TestBed.inject(Router);
    spyOn(router, 'navigate').and.returnValue(Promise.resolve(true));
    fixture = TestBed.createComponent(SellerEarningsComponent);
  });

  it('derives the commission percentage from commission_cents and gross_cents (or defaults to 10)', () => {
    expect(deriveCommissionPercent(sampleTransactions[0])).toBe(10);
    expect(
      deriveCommissionPercent({
        ...sampleTransactions[0],
        gross_cents: 10000,
        commission_cents: 1500,
      }),
    ).toBe(15);
    expect(
      deriveCommissionPercent({
        ...sampleTransactions[0],
        gross_cents: 0,
        commission_cents: 0,
      }),
    ).toBe(10);
  });

  it('renders the 3 summary cards, transaction breakdown rows, and empty payouts message', () => {
    mockPaymentsApi.getSellerEarnings.and.returnValue(of(sampleResponse));

    fixture.detectChanges();

    expect(
      fixture.nativeElement.querySelector('[data-testid="pending-amount"]').textContent,
    ).toContain('$340.00 USD');
    expect(
      fixture.nativeElement.querySelector('[data-testid="available-amount"]').textContent,
    ).toContain('$620.00 USD');
    expect(
      fixture.nativeElement.querySelector('[data-testid="paid-amount"]').textContent,
    ).toContain('$880.00 USD');

    const breakdownCard = fixture.nativeElement.querySelector(
      '[data-testid="transaction-breakdown-card"]',
    ) as HTMLElement;
    expect(breakdownCard).toBeTruthy();
    expect(breakdownCard.textContent).toContain('Handwoven Rattan Pendant Lamp');
    expect(breakdownCard.textContent).toContain('$60.00 USD');
    expect(breakdownCard.textContent).toContain('$8.00 USD');
    expect(breakdownCard.textContent).toContain('Commission (10%)');
    expect(breakdownCard.textContent).toContain('−$6.00 USD');
    expect(breakdownCard.textContent).toContain('$62.00 USD');

    const payoutsEmpty = fixture.nativeElement.querySelector(
      '[data-testid="payouts-empty"]',
    ) as HTMLElement;
    expect(payoutsEmpty.textContent).toContain(
      'No payouts recorded yet. Settlements are recorded by the platform.',
    );
  });

  it('requests a new range_days when the date range select changes and switches tabs between Earnings and Payout History', () => {
    mockPaymentsApi.getSellerEarnings.and.returnValue(of(sampleResponse));

    fixture.detectChanges();
    expect(mockPaymentsApi.getSellerEarnings).toHaveBeenCalledWith(30, 1, 20);

    fixture.componentInstance.onRangeChange(90);
    fixture.detectChanges();
    expect(mockPaymentsApi.getSellerEarnings).toHaveBeenCalledWith(90, 1, 20);

    // Switch to Payout History tab -> hides summary cards and shows only payout history card
    const payoutsTab = fixture.nativeElement.querySelector(
      '[data-testid="tab-payouts"]',
    ) as HTMLButtonElement;
    payoutsTab.click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('[data-testid="summary-card-pending"]')).toBeNull();
    expect(fixture.nativeElement.querySelector('[data-testid="payout-history-card"]')).toBeTruthy();
  });

  it('renders the "No earnings yet" empty state when there are no transactions', () => {
    mockPaymentsApi.getSellerEarnings.and.returnValue(
      of({
        data: {
          summary: {
            pending_cents: 0,
            available_cents: 0,
            paid_cents: 0,
            currency: 'USD',
          },
          transactions: [],
          payouts: [],
        },
        meta: { page: 1, page_size: 20, total: 0 },
      }),
    );

    fixture.detectChanges();

    const emptyEl = fixture.nativeElement.querySelector(
      '[data-testid="transactions-empty"]',
    ) as HTMLElement;
    expect(emptyEl).toBeTruthy();
    expect(emptyEl.textContent).toContain('No earnings yet');
  });

  it('displays "Seller access required" when getSellerEarnings fails with HTTP 403', () => {
    mockPaymentsApi.getSellerEarnings.and.returnValue(
      throwError(() => new PaymentsApiError(403, 'forbidden: seller role required')),
    );

    fixture.detectChanges();

    const alertEl = fixture.nativeElement.querySelector(
      '[data-testid="earnings-error-alert"]',
    ) as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.textContent).toContain('Seller access required');
  });
});
