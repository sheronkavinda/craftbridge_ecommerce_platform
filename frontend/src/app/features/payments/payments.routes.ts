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
    path: '',
    redirectTo: 'result',
    pathMatch: 'full',
  },
];
