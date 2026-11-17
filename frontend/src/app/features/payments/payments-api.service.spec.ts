import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { environment } from '../../../environments/environment';
import { PaymentsApiError, PaymentsApiService } from './payments-api.service';
import { BuyerPayment, InitiatePaymentResponse } from './payments.models';

describe('PaymentsApiService', () => {
  let service: PaymentsApiService;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(PaymentsApiService);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpMock.verify();
  });

  it('initiatePayment posts to /orders/:id/payments and unwraps the data envelope', () => {
    const mockData: InitiatePaymentResponse = {
      attempt_id: 'att-001',
      status: 'PENDING',
      checkout: {
        checkout_url: 'https://sandbox.payhere.lk/pay/checkout',
        method: 'POST',
        fields: { merchant_id: '1211149', order_id: 'att-001', amount: '115.00' },
      },
    };

    let actual: InitiatePaymentResponse | undefined;
    service.initiatePayment('order-005').subscribe((res) => (actual = res));

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/orders/order-005/payments`);
    expect(req.request.method).toBe('POST');
    req.flush({ data: mockData });

    expect(actual).toEqual(mockData);
  });

  it('getPayment fetches /me/payments/:id and unwraps the data envelope', () => {
    const mockPayment: BuyerPayment = {
      id: 'att-001',
      order_id: 'order-005',
      order_number: 'CB-1042',
      status: 'PENDING',
      amount_cents: 11500,
      amount: '115.00',
      currency: 'USD',
      provider: 'payhere_sandbox',
      created_at: '2026-11-17T10:00:00Z',
    };

    let actual: BuyerPayment | undefined;
    service.getPayment('att-001').subscribe((res) => (actual = res));

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/me/payments/att-001`);
    expect(req.request.method).toBe('GET');
    req.flush({ data: mockPayment });

    expect(actual).toEqual(mockPayment);
  });

  it('listPayments sends page, page_size, and status query params', () => {
    service.listPayments(2, 10, 'SUCCESS').subscribe((res) => {
      expect(res.meta.page).toBe(2);
      expect(res.data.length).toBe(1);
    });

    const req = httpMock.expectOne(
      (r) =>
        r.url === `${environment.apiBaseUrl}/me/payments` &&
        r.params.get('page') === '2' &&
        r.params.get('page_size') === '10' &&
        r.params.get('status') === 'SUCCESS',
    );
    expect(req.request.method).toBe('GET');
    req.flush({
      data: [
        {
          id: 'att-002',
          order_id: 'order-001',
          order_number: 'CB-1001',
          status: 'SUCCESS',
          amount_cents: 200000,
          amount: '2000.00',
          currency: 'USD',
          provider: 'payhere_sandbox',
          created_at: '2026-11-17T10:00:00Z',
        },
      ],
      meta: { page: 2, page_size: 10, total: 11 },
    });
  });

  it('maps backend {"error": "..."} responses to PaymentsApiError with status and message', () => {
    let capturedErr: unknown;
    service.initiatePayment('order-missing').subscribe({
      next: () => fail('expected error'),
      error: (err) => (capturedErr = err),
    });

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/orders/order-missing/payments`);
    req.flush({ error: 'order not found' }, { status: 404, statusText: 'Not Found' });

    expect(capturedErr instanceof PaymentsApiError).toBeTrue();
    const apiErr = capturedErr as PaymentsApiError;
    expect(apiErr.status).toBe(404);
    expect(apiErr.message).toBe('order not found');
  });
});
