import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { environment } from '../../../environments/environment';
import { AuthService } from './auth.service';

describe('AuthService', () => {
  let service: AuthService;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(AuthService);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpMock.verify();
    localStorage.clear();
  });

  it('stores the returned buyer identity and session expiry after login', () => {
    service.login('buyer@example.test', 'correct-horse').subscribe();
    const request = httpMock.expectOne(`${environment.apiBaseUrl}/auth/login`);
    request.flush({
      access_token: 'signed-session-token',
      expires_in: 3600,
      user: { id: 21, name: 'Buyer', email: 'buyer@example.test', roles: ['buyer'] },
    });

    expect(localStorage.getItem('access_token')).toBe('signed-session-token');
    expect(localStorage.getItem('user_id')).toBe('21');
    expect(localStorage.getItem('user_role')).toBe('buyer');
    expect(Number(localStorage.getItem('auth_expires_at'))).toBeGreaterThan(Date.now());
    expect(service.isLoggedIn()).toBeTrue();
  });

  it('clears local credentials once the server logout endpoint succeeds', () => {
    localStorage.setItem('access_token', 'signed-session-token');
    localStorage.setItem('user_role', 'buyer');

    service.logout().subscribe();
    const request = httpMock.expectOne(`${environment.apiBaseUrl}/auth/logout`);
    request.flush(null);

    expect(localStorage.getItem('access_token')).toBeNull();
    expect(localStorage.getItem('user_role')).toBeNull();
    expect(service.isLoggedIn()).toBeFalse();
  });
});
