import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { BuyerPayment, BuyerRefund } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import {
  RefundRequestComponent,
  parseRefundDecimalToCents,
} from './refund-request.component';

describe('RefundRequestComponent', () => {
  let fixture: ComponentFixture<RefundRequestComponent>;
  let mockPaymentsApi: jasmine.SpyObj<PaymentsApiService>;
  let queryOrderParam = '';

  const nowIso = new Date().toISOString();

  const paidOrders: BuyerPayment[] = [
    {
      id: 'att-005',
      order_id: 'order-005',
      order_number: 'CB-1005',
      status: 'SUCCESS',
      amount_cents: 200000,
      amount: '2000.00',
      currency: 'USD',
      provider: 'payhere_sandbox',
      created_at: nowIso,
      completed_at: nowIso,
    },
    {
      id: 'att-006',
      order_id: 'order-006',
      order_number: 'CB-1006',
      status: 'SUCCESS',
      amount_cents: 5000,
      amount: '50.00',
      currency: 'USD',
      provider: 'payhere_sandbox',
      created_at: nowIso,
      completed_at: nowIso,
    },
  ];

  const existingRefund: BuyerRefund = {
    id: 'ref-101',
    order_id: 'order-005',
    order_number: 'CB-1005',
    status: 'PENDING',
    reason: 'ARRIVED_DAMAGED',
    details: 'One of the ceramic mugs arrived with a cracked handle.',
    amount_cents: 50000,
    amount: '500.00',
    currency: 'USD',
    requested_at: nowIso,
    timeline: [
      { step: 'submitted', at: nowIso, done: true },
      { step: 'under_review', at: nowIso, done: true },
      { step: 'approved', done: false },
      { step: 'refund_issued', done: false },
    ],
  };

  beforeEach(async () => {
    queryOrderParam = '';
    mockPaymentsApi = jasmine.createSpyObj<PaymentsApiService>('PaymentsApiService', [
      'listPayments',
      'listRefunds',
      'createRefundRequest',
    ]);

    mockPaymentsApi.listPayments.and.returnValue(
      of({
        data: paidOrders,
        meta: { page: 1, page_size: 100, total: 2 },
      }),
    );
    mockPaymentsApi.listRefunds.and.returnValue(
      of({
        data: [existingRefund],
        meta: { page: 1, page_size: 100, total: 1 },
      }),
    );

    await TestBed.configureTestingModule({
      imports: [RefundRequestComponent],
      providers: [
        provideRouter([]),
        { provide: PaymentsApiService, useValue: mockPaymentsApi },
        {
          provide: ActivatedRoute,
          useValue: {
            get snapshot() {
              return {
                queryParamMap: convertToParamMap(
                  queryOrderParam ? { order: queryOrderParam } : {},
                ),
              };
            },
          },
        },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(RefundRequestComponent);
  });

  it('parses decimal amounts to cents without floating point and rejects invalid strings', () => {
    expect(parseRefundDecimalToCents('2000.00')).toBe(200000);
    expect(parseRefundDecimalToCents('20.5')).toBe(2050);
    expect(parseRefundDecimalToCents('0.05')).toBe(5);
    expect(parseRefundDecimalToCents('-5.00')).toBeNull();
    expect(parseRefundDecimalToCents('12.345')).toBeNull();
    expect(parseRefundDecimalToCents('abc')).toBeNull();
  });

  it('computes max refundable balance by subtracting non-REJECTED refunds and renders the disabled evidence photo block', () => {
    fixture.detectChanges();

    // Order-005 paid 200000 cents, existing PENDING refund is 50000 cents -> max refundable = 150000 ($1,500.00 USD)
    expect(fixture.componentInstance.maxRefundableCents()).toBe(150000);
    const maxHelper = fixture.nativeElement.querySelector(
      '[data-testid="max-refundable-helper"]',
    ) as HTMLElement;
    expect(maxHelper.textContent).toContain('$1,500.00 USD');

    const photoBtn = fixture.nativeElement.querySelector(
      '[data-testid="evidence-photos-disabled-btn"]',
    ) as HTMLButtonElement;
    expect(photoBtn.disabled).toBeTrue();
    expect(
      fixture.nativeElement.querySelector('[data-testid="evidence-photos-helper"]')
        .textContent,
    ).toContain('Photo uploads will be available soon.');
  });

  it('preselects the order specified by ?order= query parameter and updates the live character counter', () => {
    queryOrderParam = 'order-006';
    fixture.detectChanges();

    expect(fixture.componentInstance.selectedOrderId()).toBe('order-006');
    expect(fixture.componentInstance.maxRefundableCents()).toBe(5000);

    fixture.componentInstance.onDetailsInput('Cracked glaze along the rim of the bowl.');
    fixture.detectChanges();

    const counter = fixture.nativeElement.querySelector(
      '[data-testid="details-counter"]',
    ) as HTMLElement;
    expect(counter.textContent?.trim()).toBe('40/500');
  });

  it('reuses the same Idempotency-Key when retrying after a failed submission and regenerates it after a successful submission', () => {
    fixture.detectChanges();

    const comp = fixture.componentInstance;
    comp.onDetailsInput('One of the mugs arrived with a cracked handle.');
    comp.onAmountInput('20.00');
    const keyBeforeSubmit = comp.currentIdempotencyKey();

    // 1. Simulate transient failure -> key must remain identical for retry
    mockPaymentsApi.createRefundRequest.and.returnValue(
      throwError(() => new PaymentsApiError(500, 'server error')),
    );
    comp.submitRefundRequest();
    expect(mockPaymentsApi.createRefundRequest).toHaveBeenCalledTimes(1);
    expect(mockPaymentsApi.createRefundRequest.calls.mostRecent().args[2]).toBe(
      keyBeforeSubmit,
    );
    expect(comp.currentIdempotencyKey()).toBe(keyBeforeSubmit);

    // 2. Retry succeeds (201) -> uses the SAME key on the retry call, then rotates to a new key afterwards
    const createdRefund: BuyerRefund = {
      ...existingRefund,
      id: 'ref-new-202',
      amount_cents: 2000,
      amount: '20.00',
    };
    mockPaymentsApi.createRefundRequest.and.returnValue(
      of({ refund: createdRefund, replayed: false }),
    );
    comp.submitRefundRequest();

    expect(mockPaymentsApi.createRefundRequest).toHaveBeenCalledTimes(2);
    expect(mockPaymentsApi.createRefundRequest.calls.mostRecent().args[2]).toBe(
      keyBeforeSubmit,
    );
    expect(comp.currentIdempotencyKey()).not.toBe(keyBeforeSubmit);
    expect(comp.selectedRefund()?.id).toBe('ref-new-202');
  });

  it('maps 409 and 422 errors to accessible role="alert" messages and handles 200 replay', () => {
    fixture.detectChanges();
    const comp = fixture.componentInstance;
    comp.onDetailsInput('Package arrived damaged during transit.');
    comp.onAmountInput('20.00');

    // 409 window expired
    mockPaymentsApi.createRefundRequest.and.returnValue(
      throwError(() => new PaymentsApiError(409, 'refund window expired')),
    );
    comp.submitRefundRequest();
    fixture.detectChanges();

    const errEl = fixture.nativeElement.querySelector(
      '[data-testid="refund-submit-error"]',
    ) as HTMLElement;
    expect(errEl.textContent).toContain('30-day refund window');

    // 422 balance exceeded
    mockPaymentsApi.createRefundRequest.and.returnValue(
      throwError(() => new PaymentsApiError(422, 'amount exceeds refundable balance')),
    );
    comp.submitRefundRequest();
    fixture.detectChanges();
    expect(errEl.textContent).toContain('exceeds the remaining refundable balance');

    // 200 replay
    mockPaymentsApi.createRefundRequest.and.returnValue(
      of({ refund: existingRefund, replayed: true }),
    );
    comp.submitRefundRequest();
    fixture.detectChanges();
    expect(
      fixture.nativeElement.querySelector('[data-testid="refund-replay-banner"]')
        .textContent,
    ).toContain('You already submitted this request');
  });

  it('renders the timeline steps and status banners accurately across statuses and allows selecting from "Your requests"', () => {
    const completedRefund: BuyerRefund = {
      ...existingRefund,
      id: 'ref-completed-2',
      status: 'COMPLETED',
      decided_at: nowIso,
      completed_at: nowIso,
      timeline: [
        { step: 'submitted', at: nowIso, done: true },
        { step: 'under_review', at: nowIso, done: true },
        { step: 'approved', at: nowIso, done: true },
        { step: 'refund_issued', at: nowIso, done: true },
      ],
    };

    mockPaymentsApi.listRefunds.and.returnValue(
      of({
        data: [existingRefund, completedRefund],
        meta: { page: 1, page_size: 100, total: 2 },
      }),
    );

    fixture.detectChanges();

    // Initially first refund (PENDING) is selected
    const banner = fixture.nativeElement.querySelector(
      '[data-testid="existing-refund-status-banner"]',
    ) as HTMLElement;
    expect(banner.textContent).toContain('Under review');

    // Click second refund (COMPLETED) in "Your requests" list
    const listButtons = fixture.nativeElement.querySelectorAll(
      '[data-testid="refund-list-item"]',
    ) as NodeListOf<HTMLButtonElement>;
    expect(listButtons.length).toBe(2);
    listButtons[1].click();
    fixture.detectChanges();

    expect(banner.textContent).toContain('Refund issued');
    const doneSteps = fixture.nativeElement.querySelectorAll(
      '[data-testid="timeline-step"][data-step-done="true"]',
    );
    expect(doneSteps.length).toBe(4);
  });
});
