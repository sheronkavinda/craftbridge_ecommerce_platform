import { ComponentFixture, TestBed, fakeAsync, tick } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { LAST_PAYMENT_ATTEMPT_STORAGE_KEY } from '../payment-redirect/payment-redirect.component';
import { BuyerPayment } from '../payments.models';
import { PaymentsApiService } from '../payments-api.service';
import { PaymentResultComponent } from './payment-result.component';

describe('PaymentResultComponent', () => {
  let mockPaymentsApi: jasmine.SpyObj<PaymentsApiService>;

  const basePayment: BuyerPayment = {
    id: 'att-1042',
    order_id: 'order-005',
    order_number: 'CB-1042',
    status: 'PENDING',
    amount_cents: 11500,
    amount: '115.00',
    currency: 'USD',
    provider: 'payhere_sandbox',
    created_at: '2025-03-08T10:00:00Z',
  };

  function setupTestBed(queryParams: Record<string, string> = {}): ComponentFixture<PaymentResultComponent> {
    TestBed.configureTestingModule({
      imports: [PaymentResultComponent],
      providers: [
        provideRouter([]),
        { provide: PaymentsApiService, useValue: mockPaymentsApi },
        {
          provide: ActivatedRoute,
          useValue: {
            snapshot: {
              queryParamMap: convertToParamMap(queryParams),
            },
          },
        },
      ],
    });
    return TestBed.createComponent(PaymentResultComponent);
  }

  beforeEach(() => {
    sessionStorage.clear();
    mockPaymentsApi = jasmine.createSpyObj<PaymentsApiService>('PaymentsApiService', [
      'getPayment',
      'listPayments',
    ]);
  });

  afterEach(() => {
    sessionStorage.clear();
  });

  it('renders PENDING state matching mockup-13 and never trusts URL status query params', () => {
    mockPaymentsApi.getPayment.and.returnValue(of(basePayment));

    // Even if URL has ?status=SUCCESS, component must trust getPayment() which says PENDING
    const fixture = setupTestBed({ attempt: 'att-1042', status: 'SUCCESS' });
    fixture.detectChanges();

    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('Confirming your payment');
    expect(text).toContain('Order #CB-1042');
    expect(text).toContain('$115.00 USD');
    expect(text).toContain('Pending payment');
    expect(text).toContain('We are waiting for payment confirmation.');
    expect(text).toContain('PayHere Sandbox');
    expect(text).toContain('Order overview');
    expect(text).toContain('View order');
    expect(text).toContain('Back to shop');
    fixture.destroy();
  });

  it('polls every 3s while PENDING and stops polling immediately when status becomes SUCCESS', fakeAsync(() => {
    const successPayment: BuyerPayment = {
      ...basePayment,
      status: 'SUCCESS',
      completed_at: '2025-03-08T10:01:00Z',
    };

    mockPaymentsApi.getPayment.and.returnValues(
      of(basePayment),
      of(basePayment),
      of(successPayment),
    );

    const fixture = setupTestBed({ attempt: 'att-1042' });
    fixture.detectChanges();
    expect(mockPaymentsApi.getPayment).toHaveBeenCalledTimes(1);

    // Tick 3s -> still PENDING
    tick(3000);
    fixture.detectChanges();
    expect(mockPaymentsApi.getPayment).toHaveBeenCalledTimes(2);

    // Tick 3s -> transitions to SUCCESS
    tick(3000);
    fixture.detectChanges();
    expect(mockPaymentsApi.getPayment).toHaveBeenCalledTimes(3);
    expect(fixture.nativeElement.textContent).toContain('Payment confirmed');
    expect(fixture.nativeElement.textContent).toContain('View receipt');

    // Further ticks must NOT trigger additional getPayment calls
    tick(15_000);
    expect(mockPaymentsApi.getPayment).toHaveBeenCalledTimes(3);
    fixture.destroy();
  }));

  it('stops polling after 2 minutes and displays "This is taking longer than expected"', fakeAsync(() => {
    mockPaymentsApi.getPayment.and.returnValue(of(basePayment));

    const fixture = setupTestBed({ attempt: 'att-1042' });
    fixture.detectChanges();

    tick(120_000);
    fixture.detectChanges();

    expect(fixture.componentInstance.timedOut()).toBeTrue();
    expect(fixture.nativeElement.textContent).toContain('This is taking longer than expected');
    expect(fixture.nativeElement.textContent).toContain('Check again');

    const callCountAtTimeout = mockPaymentsApi.getPayment.calls.count();
    tick(9000);
    expect(mockPaymentsApi.getPayment.calls.count()).toBe(callCountAtTimeout);
    fixture.destroy();
  }));

  it('renders FAILED state with plain explanation and "Try payment again" button', () => {
    mockPaymentsApi.getPayment.and.returnValue(
      of({
        ...basePayment,
        status: 'FAILED',
      }),
    );

    const fixture = setupTestBed({ attempt: 'att-1042' });
    fixture.detectChanges();

    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain("Your payment didn't go through");
    expect(text).toContain('Try payment again');
    fixture.destroy();
  });

  it('resolves attempt id from sessionStorage when query params are absent', () => {
    sessionStorage.setItem(LAST_PAYMENT_ATTEMPT_STORAGE_KEY, 'att-from-session');
    mockPaymentsApi.getPayment.and.returnValue(
      of({ ...basePayment, id: 'att-from-session', status: 'SUCCESS' }),
    );

    const fixture = setupTestBed({});
    fixture.detectChanges();

    expect(mockPaymentsApi.getPayment).toHaveBeenCalledOnceWith('att-from-session');
    expect(mockPaymentsApi.listPayments).not.toHaveBeenCalled();
    fixture.destroy();
  });

  it('falls back to listPayments(1, 1) when both query params and sessionStorage are absent', () => {
    mockPaymentsApi.listPayments.and.returnValue(
      of({
        data: [{ ...basePayment, id: 'att-from-list', status: 'SUCCESS' }],
        meta: { page: 1, page_size: 1, total: 1 },
      }),
    );
    mockPaymentsApi.getPayment.and.returnValue(
      of({ ...basePayment, id: 'att-from-list', status: 'SUCCESS' }),
    );

    const fixture = setupTestBed({});
    fixture.detectChanges();

    expect(mockPaymentsApi.listPayments).toHaveBeenCalledOnceWith(1, 1);
    expect(mockPaymentsApi.getPayment).toHaveBeenCalledOnceWith('att-from-list');
    fixture.destroy();
  });
});
