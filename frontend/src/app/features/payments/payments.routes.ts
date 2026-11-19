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
    path: '',
    redirectTo: 'history',
    pathMatch: 'full',
  },
];
