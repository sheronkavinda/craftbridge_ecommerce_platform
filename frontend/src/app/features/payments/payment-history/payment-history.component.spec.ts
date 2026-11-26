import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { BuyerPayment } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { AccountApiService } from '../../account/account-api.service';
import { PaymentHistoryComponent } from './payment-history.component';

describe('PaymentHistoryComponent', () => {
  let fixture: ComponentFixture<PaymentHistoryComponent>;
  let mockPaymentsApi: jasmine.SpyObj<PaymentsApiService>;
  let router: Router;

  const samplePayments: BuyerPayment[] = [
    {
      id: 'att-paid-1',
      order_id: 'order-001',
      order_number: 'CB-1001',
      status: 'SUCCESS',
      amount_cents: 200000,
      amount: '2000.00',
      currency: 'USD',
      provider: 'payhere_sandbox',
      created_at: '2026-11-15T10:00:00Z',
    },
    {
      id: 'att-pending-2',
      order_id: 'order-002',
      order_number: 'CB-1002',
      status: 'PENDING',
      amount_cents: 11500,
      amount: '115.00',
      currency: 'USD',
      provider: 'payhere_sandbox',
      created_at: '2026-11-16T10:00:00Z',
    },
    {
      id: 'att-failed-3',
      order_id: 'order-003',
      order_number: 'CB-1003',
      status: 'FAILED',
      amount_cents: 4550,
      amount: '45.50',
      currency: 'USD',
      provider: 'custom_gateway',
      created_at: '2026-11-17T10:00:00Z',
    },
  ];

  beforeEach(async () => {
    mockPaymentsApi = jasmine.createSpyObj<PaymentsApiService>('PaymentsApiService', [
      'listPayments',
    ]);

    await TestBed.configureTestingModule({
      imports: [PaymentHistoryComponent],
      providers: [
        provideRouter([]),
        { provide: PaymentsApiService, useValue: mockPaymentsApi },
        {
          provide: AccountApiService,
          useValue: {
            me: () =>
              of({ id: 1, name: 'Nimali Perera', email: 'nimali@example.com', roles: ['buyer'] }),
          },
        },
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
    fixture = TestBed.createComponent(PaymentHistoryComponent);
  });

  it('renders desktop table rows, mobile cards, and per-status action links (View receipt, Check status + Pay now, Try again)', () => {
    mockPaymentsApi.listPayments.and.returnValue(
      of({
        data: samplePayments,
        meta: { page: 1, page_size: 10, total: 3 },
      }),
    );

    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('[data-testid="payment-table-row"]');
    expect(rows.length).toBe(3);

    const mobileCards = fixture.nativeElement.querySelectorAll(
      '[data-testid="payment-mobile-card"]',
    );
    expect(mobileCards.length).toBe(3);

    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('#CB-1001');
    expect(text).toContain('$2,000.00 USD');
    expect(text).toContain('PayHere Sandbox');
    expect(text).toContain('custom_gateway');
    expect(text).toContain('Showing 1-3 of 3');

    expect(fixture.nativeElement.querySelector('[data-testid="action-view-receipt"]')).toBeTruthy();
    expect(fixture.nativeElement.querySelector('[data-testid="action-check-status"]')).toBeTruthy();
    expect(fixture.nativeElement.querySelector('[data-testid="action-pay-now"]')).toBeTruthy();
    expect(fixture.nativeElement.querySelector('[data-testid="action-try-again"]')).toBeTruthy();
  });

  it('updates query string and requests filtered status when a filter tab is clicked', () => {
    mockPaymentsApi.listPayments.and.returnValue(
      of({
        data: [samplePayments[0]],
        meta: { page: 1, page_size: 10, total: 1 },
      }),
    );

    fixture.detectChanges();
    expect(mockPaymentsApi.listPayments).toHaveBeenCalledWith(1, 10, undefined);

    const paidTab = fixture.nativeElement.querySelector(
      '[data-testid="status-filter-tab"][data-filter-value="SUCCESS"]',
    ) as HTMLButtonElement;
    paidTab.click();
    fixture.detectChanges();

    expect(mockPaymentsApi.listPayments).toHaveBeenCalledWith(1, 10, 'SUCCESS');
    expect(router.navigate).toHaveBeenCalled();
  });

  it('navigates between pages and updates range text when Next/Previous are clicked', () => {
    mockPaymentsApi.listPayments.and.returnValues(
      of({
        data: samplePayments,
        meta: { page: 1, page_size: 10, total: 25 },
      }),
      of({
        data: samplePayments,
        meta: { page: 2, page_size: 10, total: 25 },
      }),
    );

    fixture.detectChanges();

    const prevBtn = fixture.nativeElement.querySelector(
      '[data-testid="pagination-prev"]',
    ) as HTMLButtonElement;
    const nextBtn = fixture.nativeElement.querySelector(
      '[data-testid="pagination-next"]',
    ) as HTMLButtonElement;

    expect(prevBtn.disabled).toBeTrue();
    expect(nextBtn.disabled).toBeFalse();

    nextBtn.click();
    fixture.detectChanges();

    expect(mockPaymentsApi.listPayments).toHaveBeenCalledWith(2, 10, undefined);
    expect(fixture.nativeElement.textContent).toContain('Showing 11-13 of 25');
    expect(prevBtn.disabled).toBeFalse();
  });

  it('renders the empty state when no payments exist', () => {
    mockPaymentsApi.listPayments.and.returnValue(
      of({
        data: [],
        meta: { page: 1, page_size: 10, total: 0 },
      }),
    );

    fixture.detectChanges();

    const emptyEl = fixture.nativeElement.querySelector(
      '[data-testid="payments-empty"]',
    ) as HTMLElement;
    expect(emptyEl).toBeTruthy();
    expect(emptyEl.textContent).toContain('No payments yet');
    expect(emptyEl.textContent).toContain('Back to shop');
  });

  it('renders an accessible error alert with a Try again button when listPayments fails', () => {
    mockPaymentsApi.listPayments.and.returnValue(
      throwError(() => new PaymentsApiError(500, 'failed to list buyer payments')),
    );

    fixture.detectChanges();

    const alertEl = fixture.nativeElement.querySelector(
      '[data-testid="payments-error"]',
    ) as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.getAttribute('role')).toBe('alert');
    expect(alertEl.textContent).toContain('failed to list buyer payments');
  });
});
