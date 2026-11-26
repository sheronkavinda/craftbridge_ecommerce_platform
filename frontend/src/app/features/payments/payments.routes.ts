import { Routes } from '@angular/router';
import { authGuard } from '../../core/guards/auth.guard';
import { roleGuard } from '../../core/guards/role.guard';

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
    canActivate: [authGuard],
    loadComponent: () =>
      import('./payment-history/payment-history.component').then(
        (m) => m.PaymentHistoryComponent,
      ),
  },
  {
    path: 'receipt/:orderId',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./receipt/receipt.component').then((m) => m.ReceiptComponent),
  },
  {
    path: 'refunds',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./refund-request/refund-request.component').then(
        (m) => m.RefundRequestComponent,
      ),
  },
  {
    path: 'seller-earnings',
    canActivate: [roleGuard('seller')],
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
