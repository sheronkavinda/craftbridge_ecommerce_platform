import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { environment } from '../../../environments/environment';
import { authInterceptor } from './auth.interceptor';

describe('authInterceptor', () => {
  let http: HttpClient;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    localStorage.clear();
    TestBed.configureTestingModule({
      providers: [
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

  it('adds admin dev actor headers for /admin/ API requests', () => {
    http.get(`${environment.apiBaseUrl}/admin/dashboard`).subscribe();

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/admin/dashboard`);
    expect(req.request.headers.get('X-Actor-Role')).toBe('admin');
    expect(req.request.headers.get('X-Actor-ID')).toBe('admin-001');
    expect(req.request.headers.get('X-Actor-Name')).toBe('System Admin');
    req.flush({});
  });

  it('adds buyer dev actor headers for /me/ and /orders/ API requests', () => {
    http.get(`${environment.apiBaseUrl}/me/payments`).subscribe();
    const meReq = httpMock.expectOne(`${environment.apiBaseUrl}/me/payments`);
    expect(meReq.request.headers.get('X-Actor-Role')).toBe('buyer');
    expect(meReq.request.headers.get('X-Actor-ID')).toBe('buyer-003');
    expect(meReq.request.headers.get('X-Actor-Name')).toBe('Ruwan Fernando');
    meReq.flush({});

    http.post(`${environment.apiBaseUrl}/orders/order-005/payments`, {}).subscribe();
    const orderReq = httpMock.expectOne(`${environment.apiBaseUrl}/orders/order-005/payments`);
    expect(orderReq.request.headers.get('X-Actor-Role')).toBe('buyer');
    expect(orderReq.request.headers.get('X-Actor-ID')).toBe('buyer-003');
    orderReq.flush({});
  });

  it('adds seller dev actor headers for /seller/ API requests', () => {
    http.get(`${environment.apiBaseUrl}/seller/earnings`).subscribe();

    const req = httpMock.expectOne(`${environment.apiBaseUrl}/seller/earnings`);
    expect(req.request.headers.get('X-Actor-Role')).toBe('seller');
    expect(req.request.headers.get('X-Actor-ID')).toBe('seller-u3');
    req.flush({});
  });

  it('does not attach actor headers to non-API URLs', () => {
    http.get('https://example.com/public-config.json').subscribe();

    const req = httpMock.expectOne('https://example.com/public-config.json');
    expect(req.request.headers.has('X-Actor-Role')).toBeFalse();
    expect(req.request.headers.has('X-Actor-ID')).toBeFalse();
    req.flush({});
  });
});
