import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { environment } from '../../../environments/environment';
import { AccountApiService } from './account-api.service';

describe('AccountApiService', () => {
  let service: AccountApiService;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(AccountApiService);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  it('uses the authenticated account endpoints for profile and address operations', () => {
    service.me().subscribe((profile) => expect(profile.id).toBe(17));
    httpMock.expectOne(`${environment.apiBaseUrl}/auth/me`).flush({
      id: 17,
      name: 'Adeesha',
      email: 'adeesha@example.test',
      roles: ['buyer'],
    });

    service.listAddresses().subscribe((response) => expect(response.addresses.length).toBe(0));
    const request = httpMock.expectOne(`${environment.apiBaseUrl}/me/addresses`);
    expect(request.request.method).toBe('GET');
    request.flush({ addresses: [] });
  });

  it('sends logout to the server session revocation endpoint', () => {
    service.logout().subscribe();
    const request = httpMock.expectOne(`${environment.apiBaseUrl}/auth/logout`);
    expect(request.request.method).toBe('POST');
    request.flush(null);
  });
});
