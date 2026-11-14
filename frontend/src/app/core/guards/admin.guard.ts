import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { environment } from '../../../environments/environment';

export const adminGuard: CanActivateFn = () => {
  const router = inject(Router);
  const rawRole = (localStorage.getItem('user_role') ?? '').trim();

  if (!rawRole && !environment.production) {
    return true;
  }

  if (rawRole.toLowerCase() === 'admin') {
    return true;
  }

  return router.createUrlTree(['/']);
};
