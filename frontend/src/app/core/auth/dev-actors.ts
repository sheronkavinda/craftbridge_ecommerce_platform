// Demo identities retained for legacy payment-shell display only; never used for authentication.

export interface DevActor {
  role: 'admin' | 'buyer' | 'seller';
  id: string;
  name: string;
  email: string;
}

export const ADMIN_DEV_ACTOR: DevActor = {
  role: 'admin',
  id: 'admin-001',
  name: 'System Admin',
  email: 'admin@craftbridge.test',
};
export const BUYER_DEV_ACTOR: DevActor = {
  role: 'buyer',
  id: 'buyer-003',
  name: 'Ruwan Fernando',
  email: 'ruwan@craftbridge.test',
};

export const SELLER_DEV_ACTOR: DevActor = {
  role: 'seller',
  id: 'seller-u3',
  name: 'Kasun Pottery',
  email: 'kasun@craftbridge.test',
};
