import { expect, test } from '@playwright/test';

const API_BASE = '**/api/v1';

test.describe('Member 01 — Authentication and customer account', () => {
  test('signs in and stores the authenticated identity', async ({ page }) => {
    await page.route(`${API_BASE}/auth/login`, async (route) => {
      expect(route.request().method()).toBe('POST');
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: {
          access_token: 'account-e2e-token',
          expires_in: 3600,
          user: {
            id: 17,
            name: 'Adeesha Perera',
            email: 'adeesha@example.test',
            roles: ['buyer'],
          },
        },
      });
    });

    await page.goto('/login');
    await page.locator('input[formcontrolname="email"]').fill('adeesha@example.test');
    await page.locator('input[formcontrolname="password"]').fill('correct-horse');
    await page.getByRole('button', { name: 'Sign in' }).click();

    await expect(page).toHaveURL('/');
    const stored = await page.evaluate(() => ({
      token: localStorage.getItem('access_token'),
      id: localStorage.getItem('user_id'),
      role: localStorage.getItem('user_role'),
    }));
    expect(stored.token).toBe('account-e2e-token');
    expect(stored.id).toBe('17');
    expect(stored.role).toBe('buyer');
  });

  test('loads and updates a profile, then revokes the session on sign out', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => {
      localStorage.setItem('access_token', 'buyer-profile-token');
      localStorage.setItem('user_id', '17');
      localStorage.setItem('user_role', 'buyer');
    });

    const bearerHeaders: string[] = [];
    await page.route(`${API_BASE}/auth/me`, async (route) => {
      bearerHeaders.push(route.request().headers()['authorization'] ?? '');
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: { id: 17, name: 'Adeesha Perera', email: 'adeesha@example.test', roles: ['buyer'] },
      });
    });
    await page.route(`${API_BASE}/me`, async (route) => {
      bearerHeaders.push(route.request().headers()['authorization'] ?? '');
      await route.fulfill({ status: 200, contentType: 'application/json', json: { message: 'Profile updated' } });
    });
    await page.route(`${API_BASE}/auth/logout`, async (route) => {
      bearerHeaders.push(route.request().headers()['authorization'] ?? '');
      await route.fulfill({ status: 204 });
    });

    await page.goto('/account/profile');
    await page.locator('input[formcontrolname="name"]').fill('Adeesha Updated');
    await page.getByRole('button', { name: 'Save changes' }).click();
    await expect(page.getByText('Profile updated')).toBeVisible();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page).toHaveURL(/\/login$/);

    const token = await page.evaluate(() => localStorage.getItem('access_token'));
    expect(token === null).toBeTruthy();
    expect(bearerHeaders.length > 0).toBeTruthy();
    for (const header of bearerHeaders) {
      expect(header).toBe('Bearer buyer-profile-token');
    }
  });

  test('protects account pages and performs owner-scoped address CRUD requests', async ({ page }) => {
    await page.goto('/account/profile');
    await expect(page).toHaveURL(/\/login$/);

    await page.goto('/');
    await page.evaluate(() => {
      localStorage.setItem('access_token', 'buyer-e2e-token');
      localStorage.setItem('user_id', '17');
      localStorage.setItem('user_role', 'buyer');
    });

    let address: Record<string, unknown> | null = {
      id: 44,
      label: 'Home',
      full_name: 'Adeesha Perera',
      phone: '',
      address_line1: '12 Craft Lane',
      address_line2: '',
      city: 'Austin',
      state: 'TX',
      postal_code: '78701',
      country_code: 'US',
      is_default: true,
    };
    const bearerHeaders: string[] = [];

    await page.route(`${API_BASE}/me/addresses`, async (route) => {
      bearerHeaders.push(route.request().headers()['authorization'] ?? '');
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          json: { addresses: address ? [address] : [] },
        });
        return;
      }
      if (route.request().method() === 'POST') {
        address = { ...(address ?? {}), ...(route.request().postDataJSON() as Record<string, unknown>) };
        await route.fulfill({ status: 201, contentType: 'application/json', json: address });
        return;
      }
      await route.abort();
    });
    await page.route(`${API_BASE}/me/addresses/44`, async (route) => {
      bearerHeaders.push(route.request().headers()['authorization'] ?? '');
      if (route.request().method() === 'PATCH') {
        address = { ...(address ?? {}), ...(route.request().postDataJSON() as Record<string, unknown>) };
        await route.fulfill({ status: 200, contentType: 'application/json', json: address });
        return;
      }
      if (route.request().method() === 'DELETE') {
        address = null;
        await route.fulfill({ status: 204 });
        return;
      }
      await route.abort();
    });

    await page.goto('/account/addresses');
    await expect(page.getByText('12 Craft Lane')).toBeVisible();
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('input[formcontrolname="address_line1"]').fill('14 Craft Lane');
    await page.getByRole('button', { name: 'Update address' }).click();
    await expect(page.getByText('Address updated.')).toBeVisible();

    await page.getByRole('button', { name: 'Remove' }).click();
    await expect(page.getByText('Address removed.')).toBeVisible();
    await expect(page.getByText('14 Craft Lane')).toBeHidden();
    expect(bearerHeaders.length > 0).toBeTruthy();
    for (const header of bearerHeaders) {
      expect(header).toBe('Bearer buyer-e2e-token');
    }
  });
});
