import { CanActivateFn, Router } from '@angular/router';
import { inject } from '@angular/core';
import { AuthService } from '../auth/auth.service';

export const roleGuard = (allowedRole: 'buyer' | 'seller' | 'admin'): CanActivateFn =>
  () => {
    const auth = inject(AuthService);
    const router = inject(Router);
    return auth.isLoggedIn() && auth.role() === allowedRole
      ? true
      : router.createUrlTree(['/']);
  };
