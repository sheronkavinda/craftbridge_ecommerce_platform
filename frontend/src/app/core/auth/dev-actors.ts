// DEV ONLY - replace with real auth once Member 01's authentication module is merged.

export interface DevActor {
  role: 'admin' | 'buyer' | 'seller';
  id: string;
  name: string;
}

export const ADMIN_DEV_ACTOR: DevActor = {
  role: 'admin',
  id: 'admin-001',
  name: 'System Admin',
};

export const BUYER_DEV_ACTOR: DevActor = {
  role: 'buyer',
  id: 'buyer-003',
  name: 'Ruwan Fernando',
};

export const SELLER_DEV_ACTOR: DevActor = {
  role: 'seller',
  id: 'seller-u3',
  name: 'Kasun Pottery',
};

/**
 * Selects a development actor identity based on the target API path so Admin,
 * Buyer, and Seller screens work out-of-the-box before real JWT login is wired.
 */
export function resolveDevActorForUrl(url: string): DevActor {
  if (url.includes('/admin/') || url.includes('/reports/') || url.includes('/audit/')) {
    return ADMIN_DEV_ACTOR;
  }
  if (url.includes('/me/') || url.includes('/orders/')) {
    return BUYER_DEV_ACTOR;
  }
  if (url.includes('/seller/')) {
    return SELLER_DEV_ACTOR;
  }
  return ADMIN_DEV_ACTOR;
}
