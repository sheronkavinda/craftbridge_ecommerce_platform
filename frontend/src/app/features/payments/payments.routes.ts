import { Routes } from '@angular/router';

export const PAYMENTS_ROUTES: Routes = [
  {
    path: 'redirect/:orderId',
    loadComponent: () =>
      import('./payment-redirect/payment-redirect.component').then(
        (m) => m.PaymentRedirectComponent,
      ),
  },
  {
    path: 'result',
    loadComponent: () =>
      import('./payment-result/payment-result.component').then(
        (m) => m.PaymentResultComponent,
      ),
  },
  {
    path: 'history',
    loadComponent: () =>
      import('./payment-history/payment-history.component').then(
        (m) => m.PaymentHistoryComponent,
      ),
  },
  {
    path: 'receipt/:orderId',
    loadComponent: () =>
      import('./receipt/receipt.component').then((m) => m.ReceiptComponent),
  },
  {
    path: 'refunds',
    loadComponent: () =>
      import('./refund-request/refund-request.component').then(
        (m) => m.RefundRequestComponent,
      ),
  },
  {
    path: 'seller-earnings',
    loadComponent: () =>
      import('./seller-earnings/seller-earnings.component').then(
        (m) => m.SellerEarningsComponent,
      ),
  },
  {
    path: '',
    redirectTo: 'history',
    pathMatch: 'full',
  },
];
