import { expect, test } from '@playwright/test';

const API_BASE = '**/api/v1';

test.describe('Member 05 — Payments, Refunds & Receipts E2E', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('access_token', 'e2e-buyer-token');
      localStorage.setItem('user_id', '42');
      localStorage.setItem('user_name', 'Nimali Perera');
      localStorage.setItem('user_role', 'buyer');
    });

    await page.route(`${API_BASE}/auth/me`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          id: 42,
          name: 'Nimali Perera',
          email: 'nimali@example.com',
          roles: ['buyer'],
        },
      });
    });
  });

  test('Payment redirect page (/payments/redirect/order-005): renders checkout details and builds hidden PayHere POST form with action, merchant_id, and hash', async ({
    page,
  }) => {
    await page.route(`${API_BASE}/orders/order-005/payments`, async (route) => {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        json: {
          data: {
            attempt_id: 'att-e2e-005',
            status: 'PENDING',
            checkout: {
              checkout_url: 'https://sandbox.payhere.lk/pay/checkout',
              method: 'POST',
              fields: {
                merchant_id: '1211149',
                return_url: 'http://localhost:4200/payments/result',
                cancel_url: 'http://localhost:4200/payments/result',
                notify_url: 'http://localhost:8080/api/v1/webhooks/payments/payhere_sandbox',
                order_id: 'att-e2e-005',
                items: 'Order CB-1005',
                currency: 'USD',
                amount: '2000.00',
                first_name: 'Ruwan',
                last_name: 'Fernando',
                email: 'ruwan@craftbridge.test',
                phone: '0000000000',
                address: 'N/A',
                city: 'Colombo',
                country: 'Sri Lanka',
                hash: 'E2E_UPPER_MD5_HASH_VALUE_987654',
              },
            },
          },
        },
      });
    });

    await page.goto('/payments/redirect/order-005');

    await expect(page.getByText('Order #order-005')).toBeVisible();
    await expect(page.getByText('$2,000.00 USD')).toBeVisible();
    await expect(page.getByText('Hosted payment • Sandbox demo')).toBeVisible();

    const continueBtn = page.getByTestId('continue-to-payhere-btn');
    await expect(continueBtn).toBeVisible();

    // Prevent browser navigation on form.submit() so we can inspect the generated DOM form fields
    await page.evaluate(() => {
      HTMLFormElement.prototype.submit = function () {
        this.setAttribute('data-submitted', 'true');
      };
    });

    await continueBtn.click();

    const hiddenForm = page.locator('form[action="https://sandbox.payhere.lk/pay/checkout"]');
    await expect(hiddenForm).toHaveAttribute('data-submitted', 'true');
    await expect(
      hiddenForm.locator('input[type="hidden"][name="merchant_id"]'),
    ).toHaveAttribute('value', '1211149');
    await expect(
      hiddenForm.locator('input[type="hidden"][name="order_id"]'),
    ).toHaveAttribute('value', 'att-e2e-005');
    await expect(
      hiddenForm.locator('input[type="hidden"][name="hash"]'),
    ).toHaveAttribute('value', 'E2E_UPPER_MD5_HASH_VALUE_987654');
  });

  test('Payment result page (/payments/result): renders SUCCESS state and FAILED/CANCELLED states from verified backend status', async ({
    page,
  }) => {
    await page.route(`${API_BASE}/me/payments/att-success-1`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: {
            id: 'att-success-1',
            order_id: 'order-001',
            order_number: 'CB-1042',
            status: 'SUCCESS',
            amount_cents: 11500,
            amount: '115.00',
            currency: 'USD',
            provider: 'payhere_sandbox',
            created_at: '2026-11-15T10:00:00Z',
            completed_at: '2026-11-15T10:01:00Z',
          },
        },
      });
    });

    await page.goto('/payments/result?attempt=att-success-1');
    await expect(page.getByText('Payment confirmed')).toBeVisible();
    await expect(page.getByText('Order #CB-1042').first()).toBeVisible();
    await expect(page.getByText('$115.00 USD').first()).toBeVisible();
    await expect(page.getByRole('link', { name: 'View receipt' })).toBeVisible();

    // Failed state
    await page.route(`${API_BASE}/me/payments/att-failed-1`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: {
            id: 'att-failed-1',
            order_id: 'order-005',
            order_number: 'CB-1005',
            status: 'FAILED',
            amount_cents: 200000,
            amount: '2000.00',
            currency: 'USD',
            provider: 'payhere_sandbox',
            public_reason: 'payment_failed',
            created_at: '2026-11-16T10:00:00Z',
          },
        },
      });
    });

    await page.goto('/payments/result?attempt=att-failed-1');
    await expect(page.getByText("Your payment didn't go through")).toBeVisible();
    await expect(page.getByRole('link', { name: 'Try payment again' })).toBeVisible();

    // Cancelled state
    await page.route(`${API_BASE}/me/payments/att-cancelled-1`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: {
            id: 'att-cancelled-1',
            order_id: 'order-005',
            order_number: 'CB-1005',
            status: 'CANCELLED',
            amount_cents: 200000,
            amount: '2000.00',
            currency: 'USD',
            provider: 'payhere_sandbox',
            public_reason: 'cancelled',
            created_at: '2026-11-16T10:00:00Z',
          },
        },
      });
    });

    await page.goto('/payments/result?attempt=att-cancelled-1');
    await expect(page.getByText('Payment was cancelled')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Try payment again' })).toBeVisible();
  });

  test('Payment history (/payments/history): lists payments, filters by status, and navigates to receipt', async ({
    page,
  }) => {
    let lastRequestedUrl = '';

    await page.route(`${API_BASE}/me/payments*`, async (route) => {
      lastRequestedUrl = route.request().url();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: [
            {
              id: 'att-hist-1',
              order_id: 'order-001',
              order_number: 'CB-1001',
              status: 'SUCCESS',
              amount_cents: 200000,
              amount: '2000.00',
              currency: 'USD',
              provider: 'payhere_sandbox',
              created_at: '2026-11-15T10:00:00Z',
            },
          ],
          meta: { page: 1, page_size: 10, total: 1 },
        },
      });
    });

    await page.route(`${API_BASE}/me/orders/order-001/receipt`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: {
            receipt_number: 'RCP-20261115-A1B2C3',
            issued_at: '2026-11-15T10:02:00Z',
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
          },
        },
      });
    });

    await page.goto('/payments/history');

    const row = page.getByTestId('payment-table-row').first();
    await expect(row).toContainText('#CB-1001');
    await expect(row).toContainText('$2,000.00 USD');
    await expect(row).toContainText('PayHere Sandbox');

    // Click "Paid" filter tab -> requests status=SUCCESS
    const paidTab = page.locator('[data-testid="status-filter-tab"][data-filter-value="SUCCESS"]');
    await paidTab.click();
    expect(lastRequestedUrl).toContain('status=SUCCESS');

    // Click "View receipt" link -> navigates to /payments/receipt/order-001
    await page.getByTestId('action-view-receipt').first().click();
    await expect(page).toHaveURL(/\/payments\/receipt\/order-001$/);
    await expect(page.getByTestId('receipt-number')).toContainText('RCP-20261115-A1B2C3');
  });

  test('Receipt page (/payments/receipt/:orderId): renders receipt number and download button, and displays 409 "not paid yet" error state', async ({
    page,
  }) => {
    await page.route(`${API_BASE}/me/orders/order-001/receipt`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: {
            receipt_number: 'RCP-20261115-A1B2C3',
            issued_at: '2026-11-15T10:02:00Z',
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
          },
        },
      });
    });

    await page.goto('/payments/receipt/order-001');
    await expect(page.getByTestId('receipt-number')).toContainText('RCP-20261115-A1B2C3');
    await expect(page.getByTestId('download-receipt-btn')).toBeVisible();
    await expect(page.getByTestId('print-receipt-btn')).toBeVisible();

    // 409 unpaid order state
    await page.route(`${API_BASE}/me/orders/order-unpaid/receipt`, async (route) => {
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        json: {
          error: 'no verified payment for this order',
        },
      });
    });

    await page.goto('/payments/receipt/order-unpaid');
    await expect(page.getByTestId('receipt-error-alert')).toContainText(
      "This order doesn't have a verified payment yet. If you just paid, your payment may still be confirming.",
    );
    await expect(page.getByTestId('receipt-check-payment-link')).toBeVisible();
  });

  test('Refund request page (/payments/refunds): validates details length and refundable balance, then submits and shows Under review banner and timeline', async ({
    page,
  }) => {
    const nowIso = new Date().toISOString();

    await page.route(`${API_BASE}/me/payments*`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: [
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
          ],
          meta: { page: 1, page_size: 100, total: 1 },
        },
      });
    });

    await page.route(`${API_BASE}/me/refunds*`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: [],
          meta: { page: 1, page_size: 100, total: 0 },
        },
      });
    });

    await page.goto('/payments/refunds');

    // Validation: short details (< 10 chars) and amount > refundable balance ($2,000.00)
    await page.getByTestId('refund-details-textarea').fill('too short');
    await page.getByTestId('refund-amount-input').fill('2500.00');
    await page.getByTestId('submit-refund-btn').click();

    await expect(page.getByTestId('details-error')).toContainText(
      'Details must be at least 10 characters.',
    );
    await expect(page.getByTestId('amount-error')).toContainText(
      'Amount cannot exceed the refundable balance of $2,000.00 USD.',
    );

    // Valid submission
    await page.route(`${API_BASE}/orders/order-005/refund-requests`, async (route) => {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        json: {
          data: {
            id: 'ref-e2e-101',
            order_id: 'order-005',
            order_number: 'CB-1005',
            status: 'PENDING',
            reason: 'ARRIVED_DAMAGED',
            details: 'One of the ceramic mugs arrived with a cracked handle.',
            amount_cents: 2000,
            amount: '20.00',
            currency: 'USD',
            requested_at: nowIso,
            timeline: [
              { step: 'submitted', at: nowIso, done: true },
              { step: 'under_review', at: nowIso, done: true },
              { step: 'approved', done: false },
              { step: 'refund_issued', done: false },
            ],
          },
        },
      });
    });

    await page
      .getByTestId('refund-details-textarea')
      .fill('One of the ceramic mugs arrived with a cracked handle.');
    await page.getByTestId('refund-amount-input').fill('20.00');
    await page.getByTestId('submit-refund-btn').click();

    await expect(page.getByTestId('refund-success-banner')).toBeVisible();
    await expect(page.getByTestId('existing-refund-status-banner')).toContainText('Under review');
    await expect(page.getByTestId('refund-timeline')).toContainText('Request submitted');
    await expect(page.getByTestId('refund-timeline')).toContainText('Refund issued');
  });

  test('Seller earnings page (/payments/seller-earnings): renders Pending/Available/Paid cards, transaction breakdown, and empty payout history message', async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem('access_token', 'e2e-seller-token');
      localStorage.setItem('user_id', '77');
      localStorage.setItem('user_name', 'Amaya Crafts');
      localStorage.setItem('user_role', 'seller');
    });

    await page.route(`${API_BASE}/auth/me`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          id: 77,
          name: 'Amaya Crafts',
          email: 'amaya@example.com',
          roles: ['seller'],
        },
      });
    });

    await page.route(`${API_BASE}/seller/earnings*`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          data: {
            summary: {
              pending_cents: 34000,
              available_cents: 62000,
              paid_cents: 88000,
              currency: 'USD',
            },
            transactions: [
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
            ],
            payouts: [],
          },
          meta: { page: 1, page_size: 20, total: 1 },
        },
      });
    });

    await page.goto('/payments/seller-earnings');

    await expect(page.getByTestId('pending-amount')).toContainText('$340.00 USD');
    await expect(page.getByTestId('available-amount')).toContainText('$620.00 USD');
    await expect(page.getByTestId('paid-amount')).toContainText('$880.00 USD');

    const breakdown = page.getByTestId('transaction-breakdown-card');
    await expect(breakdown).toContainText('Handwoven Rattan Pendant Lamp');
    await expect(page.getByTestId('breakdown-merchandise')).toContainText('$60.00 USD');
    await expect(page.getByTestId('breakdown-shipping')).toContainText('$8.00 USD');
    await expect(page.getByTestId('breakdown-commission')).toContainText('Commission (10%)');
    await expect(page.getByTestId('breakdown-commission')).toContainText('−$6.00 USD');
    await expect(page.getByTestId('breakdown-refund')).toContainText('$0.00 USD');
    await expect(page.getByTestId('breakdown-net')).toContainText('$62.00 USD');

    await expect(page.getByTestId('payouts-empty')).toContainText(
      'No payouts recorded yet. Settlements are recorded by the platform.',
    );
  });
});
