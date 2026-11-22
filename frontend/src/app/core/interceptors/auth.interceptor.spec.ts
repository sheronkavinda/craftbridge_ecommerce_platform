import { HttpClient, HttpErrorResponse, provideHttpClient, withInterceptors } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { TestBed } from '@angular/core/testing';
import { environment } from '../../../environments/environment';
import { authInterceptor } from './auth.interceptor';

describe('authInterceptor', () => {
  let http: HttpClient;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideHttpClient(withInterceptors([authInterceptor])),
        provideHttpClientTesting(),
      ],
    });
    http = TestBed.inject(HttpClient);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpMock.verify();
    localStorage.clear();
  });

  it('attaches the bearer token and never trusts stored dev actor headers', () => {
    localStorage.setItem('access_token', 'valid-session-token');
    localStorage.setItem('user_role', 'admin');
    localStorage.setItem('user_id', '9001');
    localStorage.setItem('user_name', 'System Admin');

    http.get(`${environment.apiBaseUrl}/admin/dashboard`).subscribe();

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/admin/dashboard`);
    expect(req.request.headers.get('Authorization')).toBe('Bearer valid-session-token');
    expect(req.request.headers.has('X-Actor-Role')).toBeFalse();
    expect(req.request.headers.has('X-Actor-ID')).toBeFalse();
    expect(req.request.headers.has('X-Actor-Name')).toBeFalse();
    req.flush({});
  });

  it('does not send credentials to non-API URLs', () => {
    localStorage.setItem('access_token', 'valid-session-token');
    http.get('https://example.com/public-config.json').subscribe();

    const req = httpMock.expectOne('https://example.com/public-config.json');
    expect(req.request.headers.has('Authorization')).toBeFalse();
    req.flush({});
  });

  it('clears an expired session after a protected API request receives 401', () => {
    localStorage.setItem('access_token', 'expired-session-token');
    let response: unknown;

    http.get(`${environment.apiBaseUrl}/me`).subscribe({
      error: (error: HttpErrorResponse) => (response = error),
    });
    const req = httpMock.expectOne(`${environment.apiBaseUrl}/me`);
    req.flush({ error: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });

    expect(response).toBeTruthy();
    expect(localStorage.getItem('access_token')).toBeNull();
  });
});
