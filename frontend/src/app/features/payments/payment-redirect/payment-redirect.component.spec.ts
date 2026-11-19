import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { InitiatePaymentResponse } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import {
  FORM_SUBMITTER,
  LAST_PAYMENT_ATTEMPT_STORAGE_KEY,
  PaymentRedirectComponent,
} from './payment-redirect.component';

describe('PaymentRedirectComponent', () => {
  let mockPaymentsApi: jasmine.SpyObj<PaymentsApiService>;
  let formSubmitSpy: jasmine.Spy<(form: HTMLFormElement) => void>;

  const mockSession: InitiatePaymentResponse = {
    attempt_id: 'att-redirect-001',
    status: 'PENDING',
    checkout: {
      checkout_url: 'https://sandbox.payhere.lk/pay/checkout',
      method: 'POST',
      fields: {
        merchant_id: '1211149',
        order_id: 'att-redirect-001',
        amount: '2000.00',
        currency: 'USD',
        hash: 'ABCDEF123456',
      },
    },
  };

  function createComponent(orderId = 'order-005'): ComponentFixture<PaymentRedirectComponent> {
    TestBed.configureTestingModule({
      imports: [PaymentRedirectComponent],
      providers: [
        provideRouter([]),
        { provide: PaymentsApiService, useValue: mockPaymentsApi },
        { provide: FORM_SUBMITTER, useValue: formSubmitSpy },
        {
          provide: ActivatedRoute,
          useValue: {
            snapshot: {
              paramMap: convertToParamMap({ orderId }),
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(PaymentRedirectComponent);
    fixture.detectChanges();
    return fixture;
  }

  beforeEach(() => {
    sessionStorage.clear();
    mockPaymentsApi = jasmine.createSpyObj<PaymentsApiService>('PaymentsApiService', [
      'initiatePayment',
    ]);
    formSubmitSpy = jasmine.createSpy('FORM_SUBMITTER');
  });

  afterEach(() => {
    sessionStorage.clear();
  });

  it('calls initiatePayment on load, persists attempt_id, and renders "Continue to PayHere →" without auto-submitting', () => {
    mockPaymentsApi.initiatePayment.and.returnValue(of(mockSession));

    const fixture = createComponent('order-005');
    const text = fixture.nativeElement.textContent as string;

    expect(mockPaymentsApi.initiatePayment).toHaveBeenCalledOnceWith('order-005');
    expect(sessionStorage.getItem(LAST_PAYMENT_ATTEMPT_STORAGE_KEY)).toBe('att-redirect-001');
    expect(text).toContain('Order #order-005');
    expect(text).toContain('$2,000.00 USD');
    expect(text).toContain('Hosted payment • Sandbox demo');
    expect(text).toContain('Continue to PayHere →');
    expect(formSubmitSpy).not.toHaveBeenCalled();
  });

  it('builds a hidden form with the checkout action/method/fields and submits it when the button is clicked', async () => {
    mockPaymentsApi.initiatePayment.and.returnValue(of(mockSession));

    const fixture = createComponent('order-005');
    await fixture.whenStable();
    fixture.detectChanges();

    const btn = fixture.nativeElement.querySelector(
      '[data-testid="continue-to-payhere-btn"]',
    ) as HTMLButtonElement;
    expect(btn).toBeTruthy();
    btn.click();

    expect(formSubmitSpy).toHaveBeenCalledTimes(1);
    const submittedForm = formSubmitSpy.calls.mostRecent().args[0];
    expect(submittedForm).toBeDefined();
    expect(submittedForm.action).toBe('https://sandbox.payhere.lk/pay/checkout');
    expect(submittedForm.method.toUpperCase()).toBe('POST');

    const expectedFields = mockSession.checkout.fields;
    const inputs = Array.from(submittedForm.querySelectorAll('input[type="hidden"]')) as HTMLInputElement[];
    expect(inputs.length).toBe(Object.keys(expectedFields).length);

    for (const [name, expectedValue] of Object.entries(expectedFields)) {
      const input = submittedForm.querySelector(`input[name="${name}"]`) as HTMLInputElement;
      expect(input).toBeTruthy();
      expect(input.value).toBe(expectedValue);
      expect(name.toLowerCase()).not.toContain('secret');
    }

    submittedForm.remove();
  });

  it('shows accessible 404 message when order is not found', () => {
    mockPaymentsApi.initiatePayment.and.returnValue(
      throwError(() => new PaymentsApiError(404, 'order not found')),
    );
    const fixture = createComponent('order-missing');
    const alertEl = fixture.nativeElement.querySelector('[role="alert"]') as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.textContent).toContain("We couldn't find that order");
  });

  it('shows accessible 409 message when order is no longer awaiting payment', () => {
    mockPaymentsApi.initiatePayment.and.returnValue(
      throwError(() => new PaymentsApiError(409, 'order is not pending payment')),
    );
    const fixture = createComponent('order-paid');
    const alertEl = fixture.nativeElement.querySelector('[role="alert"]') as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.textContent).toContain('This order is no longer awaiting payment');
  });

  it('shows accessible 503 message when payment provider is unavailable', () => {
    mockPaymentsApi.initiatePayment.and.returnValue(
      throwError(() => new PaymentsApiError(503, 'payment provider not configured')),
    );
    const fixture = createComponent('order-005');
    const alertEl = fixture.nativeElement.querySelector('[role="alert"]') as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.textContent).toContain('Payments are temporarily unavailable');
  });
});
