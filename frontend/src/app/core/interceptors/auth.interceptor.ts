import { HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { environment } from '../../../environments/environment';
import { AuthService } from '../auth/auth.service';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  if (!req.url.startsWith(environment.apiBaseUrl)) {
    return next(req);
  }

  const token = (localStorage.getItem('access_token') ?? '').trim();
  const auth = inject(AuthService);
  const router = inject(Router);
  const authRequest = token
    ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } })
    : req;

  return next(authRequest).pipe(
    catchError((error: { status?: number }) => {
      const isAuthEntryPoint = /\/auth\/(login|register|forgot-password|reset-password)$/.test(req.url);
      if (token && error.status === 401 && !isAuthEntryPoint) {
        auth.clearSession();
        void router.navigate(['/login'], { queryParams: { reason: 'expired' } });
      }
      return throwError(() => error);
    }),
  );
};
