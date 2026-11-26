import { inject } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { CanActivateFn, Routes, Router } from '@angular/router';
import { map, catchError, of } from 'rxjs';
import { authGuard } from '../../core/guards/auth.guard';
import { SellerApi } from './seller-api.service';
const shopOwner: CanActivateFn = () => {
  const router = inject(Router);
  return inject(SellerApi)
    .shop()
    .pipe(
      map(() => true),
      catchError((e: HttpErrorResponse) => {
        if (e.status === 401) return of(router.createUrlTree(['/login']));
        return of(
          router.createUrlTree([
            e.status === 403 || e.status === 404 ? '/seller/application' : '/seller/dashboard',
          ]),
        );
      }),
    );
};
export const sellerRoutes: Routes = [
  {
    path: 'application',
    canActivate: [authGuard],
    loadComponent: () => import('./seller-application').then((m) => m.SellerApplicationPage),
  },
  {
    path: 'dashboard',
    canActivate: [authGuard],
    loadComponent: () => import('./seller-dashboard').then((m) => m.SellerDashboard),
  },
  {
    path: 'products/new',
    canActivate: [authGuard, shopOwner],
    loadComponent: () => import('./product-editor').then((m) => m.ProductEditor),
  },
  {
    path: 'products/:id',
    canActivate: [authGuard],
    loadComponent: () => import('./product-editor').then((m) => m.ProductEditor),
  },
  {
    path: 'orders',
    canActivate: [authGuard, shopOwner],
    loadComponent: () => import('./seller-orders').then((m) => m.SellerOrders),
  },
];
