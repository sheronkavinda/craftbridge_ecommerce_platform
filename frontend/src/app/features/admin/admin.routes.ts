import { Routes } from '@angular/router';
import { adminGuard } from '../../core/guards/admin.guard';
import { AdminLayoutComponent } from '../../shared/layouts/admin-layout/admin-layout.component';

export const ADMIN_ROUTES: Routes = [
  {
    path: '',
    component: AdminLayoutComponent,
    canActivate: [adminGuard],
    children: [
      {
        path: '',
        redirectTo: 'dashboard',
        pathMatch: 'full',
      },
      {
        path: 'dashboard',
        loadComponent: () =>
          import('./dashboard/admin-dashboard.component').then((m) => m.AdminDashboardComponent),
      },
      {
        path: 'users',
        loadComponent: () =>
          import('./users/users.component').then((m) => m.UsersComponent),
      },
      {
        path: 'seller-approvals',
        loadComponent: () =>
          import('./seller-approvals/seller-approvals.component').then(
            (m) => m.SellerApprovalsComponent,
          ),
      },
      {
        path: 'categories',
        loadComponent: () =>
          import('./categories/categories.component').then((m) => m.CategoriesComponent),
      },
      {
        path: 'refunds',
        loadComponent: () =>
          import('./refunds/refunds.component').then((m) => m.RefundsComponent),
      },
      {
        path: 'reports',
        loadComponent: () =>
          import('./reports/reports.component').then((m) => m.ReportsComponent),
      },
      {
        path: 'audit-log',
        loadComponent: () =>
          import('./audit-log/audit-log.component').then((m) => m.AuditLogComponent),
      },
    ],
  },
];
