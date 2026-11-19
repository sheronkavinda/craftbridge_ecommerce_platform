import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { environment } from '../../../environments/environment';
import { PaymentsApiError, PaymentsApiService } from './payments-api.service';
import {
  BuyerPayment,
  BuyerRefund,
  CreateRefundResult,
  DownloadedReceipt,
  InitiatePaymentResponse,
  ReceiptView,
  SellerEarningsResponse,
} from './payments.models';

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

  it('getReceipt fetches /me/orders/:id/receipt and unwraps the data envelope', () => {
    const mockReceipt: ReceiptView = {
      receipt_number: 'RCP-20261115-A1B2C3',
      issued_at: '2026-11-15T10:00:00Z',
      order_id: 'order-001',
      order_number: 'CB-1001',
      buyer_name: 'Ruwan Fernando',
      buyer_email: 'ruwan@craftbridge.test',
      payment_method: 'PayHere Sandbox',
      provider_reference: 'ph-001',
      amount_cents: 200000,
      amount: '2000.00',
      currency: 'USD',
      items: [],
      refunded_cents: 0,
      net_paid_cents: 200000,
      status: 'PAID',
    };

    let actual: ReceiptView | undefined;
    service.getReceipt('order-001').subscribe((res) => (actual = res));

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/me/orders/order-001/receipt`);
    expect(req.request.method).toBe('GET');
    req.flush({ data: mockReceipt });

    expect(actual).toEqual(mockReceipt);
  });

  it('downloadReceipt fetches the HTML blob and builds filename receipt-<receipt_number>.html on the client', () => {
    const htmlBlob = new Blob(['<html>Receipt</html>'], { type: 'text/html' });
    let result: DownloadedReceipt | undefined;

    service.downloadReceipt('order-001', 'RCP-20261115-A1B2C3').subscribe((res) => {
      result = res;
    });

    const req = httpMock.expectOne(
      `${environment.apiBaseUrl}/me/orders/order-001/receipt/download`,
    );
    expect(req.request.method).toBe('GET');
    expect(req.request.responseType).toBe('blob');
    req.flush(htmlBlob);

    expect(result).toBeDefined();
    expect(result!.filename).toBe('receipt-RCP-20261115-A1B2C3.html');
    expect(result!.blob.size).toBe(htmlBlob.size);
  });

  it('createRefundRequest sends Idempotency-Key header and distinguishes HTTP 201 (replayed=false) vs HTTP 200 (replayed=true)', () => {
    const mockRefund: BuyerRefund = {
      id: 'ref-001',
      order_id: 'order-005',
      order_number: 'CB-1042',
      status: 'PENDING',
      reason: 'ARRIVED_DAMAGED',
      details: 'One of the ceramic mugs arrived with a cracked handle.',
      amount_cents: 2000,
      amount: '20.00',
      currency: 'USD',
      requested_at: '2026-11-19T08:00:00Z',
    };

    let createdRes: CreateRefundResult | undefined;
    service
      .createRefundRequest(
        'order-005',
        {
          reason: 'ARRIVED_DAMAGED',
          details: 'One of the ceramic mugs arrived with a cracked handle.',
          amount: '20.00',
        },
        'idem-uuid-123',
      )
      .subscribe((res) => (createdRes = res));

    const req1 = httpMock.expectOne(
      `${environment.apiBaseUrl}/orders/order-005/refund-requests`,
    );
    expect(req1.request.method).toBe('POST');
    expect(req1.request.headers.get('Idempotency-Key')).toBe('idem-uuid-123');
    req1.flush({ data: mockRefund }, { status: 201, statusText: 'Created' });

    expect(createdRes).toEqual({ refund: mockRefund, replayed: false });

    let replayedRes: CreateRefundResult | undefined;
    service
      .createRefundRequest(
        'order-005',
        {
          reason: 'ARRIVED_DAMAGED',
          details: 'One of the ceramic mugs arrived with a cracked handle.',
          amount: '20.00',
        },
        'idem-uuid-123',
      )
      .subscribe((res) => (replayedRes = res));

    const req2 = httpMock.expectOne(
      `${environment.apiBaseUrl}/orders/order-005/refund-requests`,
    );
    req2.flush({ data: mockRefund }, { status: 200, statusText: 'OK' });

    expect(replayedRes).toEqual({ refund: mockRefund, replayed: true });
  });

  it('listRefunds and getRefund query /me/refunds and /me/refunds/:id', () => {
    service.listRefunds(1, 20).subscribe((res) => {
      expect(res.data.length).toBe(1);
      expect(res.meta.total).toBe(1);
    });

    const listReq = httpMock.expectOne(
      (r) =>
        r.url === `${environment.apiBaseUrl}/me/refunds` &&
        r.params.get('page') === '1' &&
        r.params.get('page_size') === '20',
    );
    listReq.flush({
      data: [
        {
          id: 'ref-001',
          order_id: 'order-005',
          order_number: 'CB-1042',
          status: 'PENDING',
          reason: 'ARRIVED_DAMAGED',
          details: 'Handle cracked.',
          amount_cents: 2000,
          amount: '20.00',
          currency: 'USD',
          requested_at: '2026-11-19T08:00:00Z',
        },
      ],
      meta: { page: 1, page_size: 20, total: 1 },
    });

    service.getRefund('ref-001').subscribe((ref) => {
      expect(ref.id).toBe('ref-001');
    });
    const detailReq = httpMock.expectOne(`${environment.apiBaseUrl}/me/refunds/ref-001`);
    expect(detailReq.request.method).toBe('GET');
    detailReq.flush({
      data: {
        id: 'ref-001',
        order_id: 'order-005',
        order_number: 'CB-1042',
        status: 'PENDING',
        reason: 'ARRIVED_DAMAGED',
        details: 'Handle cracked.',
        amount_cents: 2000,
        amount: '20.00',
        currency: 'USD',
        requested_at: '2026-11-19T08:00:00Z',
      },
    });
  });

  it('getSellerEarnings queries /seller/earnings with range_days, page, and page_size', () => {
    let actual: SellerEarningsResponse | undefined;
    service.getSellerEarnings(90, 2, 10).subscribe((res) => (actual = res));

    const req = httpMock.expectOne(
      (r) =>
        r.url === `${environment.apiBaseUrl}/seller/earnings` &&
        r.params.get('range_days') === '90' &&
        r.params.get('page') === '2' &&
        r.params.get('page_size') === '10',
    );
    expect(req.request.method).toBe('GET');
    req.flush({
      data: {
        summary: {
          pending_cents: 34000,
          available_cents: 62000,
          paid_cents: 88000,
          currency: 'USD',
        },
        transactions: [],
        payouts: [],
      },
      meta: { page: 2, page_size: 10, total: 15 },
    });

    expect(actual?.data.summary.available_cents).toBe(62000);
    expect(actual?.meta.page).toBe(2);
  });

  it('maps backend {"error": "..."} responses to PaymentsApiError with status and message', () => {
    let capturedErr: unknown;
    service.getReceipt('order-unpaid').subscribe({
      next: () => fail('expected error'),
      error: (err) => (capturedErr = err),
    });

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/me/orders/order-unpaid/receipt`);
    req.flush(
      { error: 'no verified payment for this order' },
      { status: 409, statusText: 'Conflict' },
    );

    expect(capturedErr instanceof PaymentsApiError).toBeTrue();
    const apiErr = capturedErr as PaymentsApiError;
    expect(apiErr.status).toBe(409);
    expect(apiErr.message).toBe('no verified payment for this order');
  });
});
