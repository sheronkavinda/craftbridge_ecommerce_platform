import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { ReceiptView } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { AccountApiService } from '../../account/account-api.service';
import { FILE_SAVER, PRINT } from '../shared/browser-actions.tokens';
import { ReceiptComponent } from './receipt.component';

describe('ReceiptComponent', () => {
  let fixture: ComponentFixture<ReceiptComponent>;
  let mockPaymentsApi: jasmine.SpyObj<PaymentsApiService>;
  let fileSaverSpy: jasmine.Spy<(blob: Blob, filename: string) => void>;
  let printSpy: jasmine.Spy<() => void>;

  const baseReceipt: ReceiptView = {
    receipt_number: 'RCP-20261115-A1B2C3',
    issued_at: '2026-11-15T10:00:00Z',
    order_id: 'order-001',
    order_number: 'CB-1001',
    buyer_name: 'Ruwan Fernando',
    buyer_email: 'ruwan@craftbridge.test',
    payment_method: 'PayHere Sandbox',
    provider_reference: 'ph-3200991',
    amount_cents: 200000,
    amount: '2000.00',
    currency: 'USD',
    items: [],
    refunded_cents: 0,
    net_paid_cents: 200000,
    status: 'PAID',
  };

  beforeEach(async () => {
    mockPaymentsApi = jasmine.createSpyObj<PaymentsApiService>('PaymentsApiService', [
      'getReceipt',
      'downloadReceipt',
    ]);
    fileSaverSpy = jasmine.createSpy('FILE_SAVER');
    printSpy = jasmine.createSpy('PRINT');

    await TestBed.configureTestingModule({
      imports: [ReceiptComponent],
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
        { provide: FILE_SAVER, useValue: fileSaverSpy },
        { provide: PRINT, useValue: printSpy },
        {
          provide: ActivatedRoute,
          useValue: {
            snapshot: {
              paramMap: convertToParamMap({ orderId: 'order-001' }),
            },
          },
        },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(ReceiptComponent);
  });

  it('renders receipt fields and hides the items table and refunded line when items are empty and refunded_cents is 0', () => {
    mockPaymentsApi.getReceipt.and.returnValue(of(baseReceipt));

    fixture.detectChanges();

    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('Official Payment Receipt');
    expect(text).toContain('RCP-20261115-A1B2C3');
    expect(text).toContain('Ruwan Fernando');
    expect(text).toContain('ruwan@craftbridge.test');
    expect(text).toContain('#CB-1001');
    expect(text).toContain('PayHere Sandbox');
    expect(text).toContain('ph-3200991');
    expect(text).toContain('$2,000.00 USD');
    expect(text).toContain('Thank you for supporting independent artisans on CraftBridge.');

    expect(fixture.nativeElement.querySelector('[data-testid="receipt-items-table"]')).toBeNull();
    expect(fixture.nativeElement.querySelector('[data-testid="receipt-refunded-row"]')).toBeNull();
  });

  it('shows the items table when items.length > 0 and shows the negative Refunded row when refunded_cents > 0', () => {
    mockPaymentsApi.getReceipt.and.returnValue(
      of({
        ...baseReceipt,
        items: [
          {
            title: 'Handwoven Palm Basket',
            quantity: 2,
            unit_price_cents: 6000,
          },
        ],
        refunded_cents: 4550,
        net_paid_cents: 195450,
      }),
    );

    fixture.detectChanges();

    const itemsTable = fixture.nativeElement.querySelector(
      '[data-testid="receipt-items-table"]',
    ) as HTMLElement;
    expect(itemsTable).toBeTruthy();
    expect(itemsTable.textContent).toContain('Handwoven Palm Basket');
    expect(itemsTable.textContent).toContain('$120.00 USD');

    const refundedRow = fixture.nativeElement.querySelector(
      '[data-testid="receipt-refunded-row"]',
    ) as HTMLElement;
    expect(refundedRow).toBeTruthy();
    expect(refundedRow.textContent).toContain('−$45.50 USD');

    const netRow = fixture.nativeElement.querySelector(
      '[data-testid="receipt-net-row"]',
    ) as HTMLElement;
    expect(netRow.textContent).toContain('$1,954.50 USD');
  });

  it('downloads the receipt via the FILE_SAVER token with filename receipt-<receipt_number>.html', () => {
    const blob = new Blob(['<html>Receipt</html>'], { type: 'text/html' });
    mockPaymentsApi.getReceipt.and.returnValue(of(baseReceipt));
    mockPaymentsApi.downloadReceipt.and.returnValue(
      of({
        blob,
        filename: 'receipt-RCP-20261115-A1B2C3.html',
      }),
    );

    fixture.detectChanges();

    const downloadBtn = fixture.nativeElement.querySelector(
      '[data-testid="download-receipt-btn"]',
    ) as HTMLButtonElement;
    downloadBtn.click();

    expect(mockPaymentsApi.downloadReceipt).toHaveBeenCalledOnceWith(
      'order-001',
      'RCP-20261115-A1B2C3',
    );
    expect(fileSaverSpy).toHaveBeenCalledOnceWith(blob, 'receipt-RCP-20261115-A1B2C3.html');
  });

  it('calls the PRINT token when the Print button is clicked', () => {
    mockPaymentsApi.getReceipt.and.returnValue(of(baseReceipt));

    fixture.detectChanges();

    const printBtn = fixture.nativeElement.querySelector(
      '[data-testid="print-receipt-btn"]',
    ) as HTMLButtonElement;
    printBtn.click();

    expect(printSpy).toHaveBeenCalledTimes(1);
  });

  it('displays the 404 message when the receipt is not found', () => {
    mockPaymentsApi.getReceipt.and.returnValue(
      throwError(() => new PaymentsApiError(404, 'order not found')),
    );

    fixture.detectChanges();

    const alertEl = fixture.nativeElement.querySelector(
      '[data-testid="receipt-error-alert"]',
    ) as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.textContent).toContain("We couldn't find a receipt for this order.");
  });

  it('displays the 409 message with a link to /payments/result when the order has no verified payment yet', () => {
    mockPaymentsApi.getReceipt.and.returnValue(
      throwError(() => new PaymentsApiError(409, 'no verified payment for this order')),
    );

    fixture.detectChanges();

    const alertEl = fixture.nativeElement.querySelector(
      '[data-testid="receipt-error-alert"]',
    ) as HTMLElement;
    expect(alertEl).toBeTruthy();
    expect(alertEl.textContent).toContain(
      "This order doesn't have a verified payment yet. If you just paid, your payment may still be confirming.",
    );
    expect(
      fixture.nativeElement.querySelector('[data-testid="receipt-check-payment-link"]'),
    ).toBeTruthy();
  });
});
