import { Routes } from '@angular/router';
import { adminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  {
    path: '',
    redirectTo: 'admin',
    pathMatch: 'full',
  },
  {
    path: 'admin',
    canActivate: [adminGuard],
    loadChildren: () =>
      import('./features/admin/admin.routes').then((m) => m.ADMIN_ROUTES),
  },
  {
    // TODO(member-01): Guard with the real buyer auth guard once Member 01's login exists.
    path: 'payments',
    loadChildren: () =>
      import('./features/payments/payments.routes').then((m) => m.PAYMENTS_ROUTES),
  },
  {
    path: '**',
    redirectTo: 'admin',
  },
];
