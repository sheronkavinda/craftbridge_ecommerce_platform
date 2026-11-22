import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../auth/auth.service';

export const adminGuard: CanActivateFn = () => {
  const router = inject(Router);
  const auth = inject(AuthService);
  if (auth.isLoggedIn() && auth.role() === 'admin') {
    return true;
  }

  return router.createUrlTree(['/login']);
};
