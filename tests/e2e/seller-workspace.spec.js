const { test, expect } = require('@playwright/test');
const { randomUUID } = require('node:crypto');
const base = process.env.M02_TEST_API || 'http://localhost:8080/api/v1';
const image = {
  name: 'bowl.png',
  mimeType: 'image/png',
  buffer: Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL4kAAAAASUVORK5CYII=',
    'base64',
  ),
};
async function applicant(page, request) {
  const adminEmail = process.env.M02_TEST_ADMIN_EMAIL;
  const adminPassword = process.env.M02_TEST_ADMIN_PASSWORD;
  test.skip(
    !adminEmail || !adminPassword,
    'Set local test admin credentials to run the database-backed journey.',
  );
  const email = 'seller-' + randomUUID() + '@example.test',
    password = 'Test-' + randomUUID();
  const reg = await request.post(base + '/auth/register', {
    data: { name: 'Journey Seller', email, password },
  });
  expect(reg.ok()).toBeTruthy();
  const headers = {
    Authorization: 'Bearer ' + (await reg.json()).access_token,
  };
  const login = await request.post(base + '/auth/login', {
    data: { email: adminEmail, password: adminPassword },
  });
  expect(login.ok()).toBeTruthy();
  const admin = {
    Authorization: 'Bearer ' + (await login.json()).access_token,
  };
  await page.goto('/login');
  await page.locator('[formcontrolname=email]').fill(email);
  await page.locator('[formcontrolname=password]').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL('/');
  await page.goto('/seller/application');
  await page.getByLabel('Store name').fill('Studio ' + randomUUID());
  await page.getByLabel(/Country code/).fill('LK');
  const result = page.waitForResponse(
    (r) =>
      r.url().endsWith('/seller/applications') &&
      r.request().method() === 'POST',
  );
  await page.getByRole('button', { name: 'Submit application' }).click();
  const id = (await (await result).json()).application.id;
  await expect(
    page.getByText('Your application is waiting for review.'),
  ).toBeVisible();
  return { admin, headers, email, id };
}
test('guests reach sign in', async ({ page }) => {
  await page.goto('/seller/dashboard');
  await expect(page).toHaveURL(/\/login$/);
});
test('rejected applicants see the review reason', async ({ page, request }) => {
  const { admin, id } = await applicant(page, request);
  const decision = await request.post(
    base + '/admin/seller-applications/' + id + '/decision',
    {
      headers: admin,
      data: { decision: 'reject', reason: 'More shop details required' },
    },
  );
  expect(decision.ok()).toBeTruthy();
  await page.goto('/seller/products/new');
  await expect(page).toHaveURL(/\/seller\/application$/);
  await expect(
    page.getByText('Review note: More shop details required'),
  ).toBeVisible();
});
test('approved seller manages profile, products, stock and media', async ({
  page,
  request,
}) => {
  test.setTimeout(90000);
  const {
    admin,
    headers,
    email,
    id: applicationID,
  } = await applicant(page, request);
  const decision = await request.post(
    base + '/admin/seller-applications/' + applicationID + '/decision',
    { headers: admin, data: { decision: 'approve' } },
  );
  expect(decision.ok()).toBeTruthy();
  const shop = (await decision.json()).shop;
  await page.reload();
  await page.getByRole('link', { name: 'Manage my shop' }).click();
  await page.getByText('Shop profile', { exact: true }).click();
  await page.getByLabel('Contact email').fill(email);
  await page.getByRole('button', { name: 'Save shop profile' }).click();
  await expect(page.getByText('Shop profile saved.')).toBeVisible();
  for (const kind of ['profile', 'banner']) {
    await page.getByLabel('Upload ' + kind + ' image').setInputFiles(image);
    await expect(
      page.getByRole('img', { name: kind + ' image' }),
    ).toBeVisible();
  }
  expect((await request.get(base + '/shops/' + shop.slug)).ok()).toBeTruthy();
  await page.getByRole('link', { name: 'Fulfillment orders' }).click();
  await expect(page.getByText('No paid orders to fulfill.')).toBeVisible();
  await page.getByRole('link', { name: 'Back to seller dashboard' }).click();
  await page.getByRole('link', { name: 'Create product' }).click();
  await page.getByLabel('Title', { exact: true }).fill('Journey clay bowl');
  await page
    .getByLabel('Description', { exact: true })
    .fill('Handmade test bowl');
  await page.getByLabel('Category').selectOption({ index: 1 });
  await page.getByLabel(/Price/).fill('12.50');
  await page.getByLabel('Opening stock').fill('5');
  await page.getByRole('button', { name: 'Create draft' }).click();
  await expect(page).toHaveURL(/\/seller\/products\/\d+$/);
  const id = new URL(page.url()).pathname.split('/').pop();
  await page.getByLabel('Image description').fill('Clay bowl');
  await page.getByLabel('Image file').setInputFiles(image);
  await page.getByRole('button', { name: 'Upload image' }).click();
  await expect(page.getByRole('img', { name: 'Clay bowl' })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('img', { name: 'Clay bowl' })).toBeVisible();
  expect(
    (await (await request.get(base + '/products/' + id + '/images')).json())
      .items,
  ).toEqual([]);
  expect(
    (
      await request.get(base + '/seller/products/' + id + '/images', {
        headers: admin,
      })
    ).status(),
  ).toBe(404);
  expect(
    (
      await request.patch(base + '/seller/products/' + id, {
        headers: admin,
        data: { title: 'Other shop edit' },
      })
    ).status(),
  ).toBe(404);
  expect(
    (
      await request.post(base + '/seller/products/' + id + '/images', {
        headers,
        multipart: {
          image: {
            name: 'note.txt',
            mimeType: 'text/plain',
            buffer: Buffer.from('plain text'),
          },
        },
      })
    ).status(),
  ).toBe(415);
  await page.getByLabel('Quantity change').fill('2');
  await page.getByLabel('Reason').fill('Kiln delivery');
  await page.getByRole('button', { name: 'Record stock adjustment' }).click();
  await expect(page.getByText('7 available · 0 reserved')).toBeVisible();
  await expect(page.getByText('Kiln delivery', { exact: true })).toBeVisible();
  await page.getByLabel('Visibility').selectOption('published');
  await page.getByRole('button', { name: 'Save product' }).click();
  await expect(page.getByText('Product saved.')).toBeVisible();
  expect(
    (await (await request.get(base + '/products/' + id)).json()).product
      .price_cents,
  ).toBe(1250);
  expect(
    (await (await request.get(base + '/products/' + id + '/images')).json())
      .items,
  ).toHaveLength(1);
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Archive product' }).click();
  await expect(page).toHaveURL(/\/seller\/dashboard$/);
  expect((await request.get(base + '/products/' + id)).status()).toBe(404);
  expect(
    (
      await (
        await request.get(base + '/seller/products/' + id, { headers })
      ).json()
    ).product.status,
  ).toBe('archived');
});
