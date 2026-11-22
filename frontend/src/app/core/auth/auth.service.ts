import { inject, Injectable } from '@angular/core';
import { Observable, catchError, tap, throwError } from 'rxjs';
import { AccountApiService } from '../../features/account/account-api.service';
import { AuthResponse } from '../../features/account/account.models';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly api = inject(AccountApiService);

  login(email: string, password: string) {
    return this.api.login({ email, password }).pipe(tap((response) => this.storeSession(response)));
  }

  register(name: string, email: string, password: string) {
    return this.api.register({ name, email, password }).pipe(tap((response) => this.storeSession(response)));
  }

  logout(): Observable<void> {
    return this.api.logout().pipe(
      tap(() => this.clearSession()),
      catchError((error: unknown) => {
        this.clearSession();
        return throwError(() => error);
      }),
    );
  }

  clearSession(): void {
    localStorage.removeItem('access_token');
    localStorage.removeItem('auth_expires_at');
    localStorage.removeItem('user_role');
    localStorage.removeItem('user_id');
    localStorage.removeItem('user_name');
  }

  isLoggedIn(): boolean {
    const token = localStorage.getItem('access_token');
    const expiresAt = Number(localStorage.getItem('auth_expires_at') ?? 0);
    if (!token || (expiresAt > 0 && expiresAt <= Date.now())) {
      this.clearSession();
      return false;
    }
    return true;
  }

  role(): string {
    return (localStorage.getItem('user_role') ?? '').toLowerCase();
  }

  private storeSession(response: AuthResponse): void {
    localStorage.setItem('access_token', response.access_token);
    localStorage.setItem('auth_expires_at', String(Date.now() + response.expires_in * 1000));
    localStorage.setItem('user_id', String(response.user.id));
    localStorage.setItem('user_name', response.user.name);
    localStorage.setItem(
      'user_role',
      response.user.roles.includes('admin')
        ? 'admin'
        : response.user.roles.includes('seller')
          ? 'seller'
          : 'buyer',
    );
  }
}
