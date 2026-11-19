
import { TestBed } from '@angular/core/testing';
import {
  provideHttpClient,
} from '@angular/common/http';
import {
  provideHttpClientTesting,
  HttpTestingController,
} from '@angular/common/http/testing';

import { StorefrontApi } from './storefront-api';
import { environment } from '../../../../environments/environment';

describe('StorefrontApi', () => {
  let service: StorefrontApi;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        StorefrontApi,
        provideHttpClient(),
        provideHttpClientTesting(),
      ],
    });

    service = TestBed.inject(StorefrontApi);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpMock.verify();
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });

  it('should request storefront health', () => {
    const mockResponse = {
      status: 'ok',
      module: 'storefront',
      message: 'Storefront API is running',
    };

    service.getHealth().subscribe((response) => {
      expect(response).toEqual(mockResponse);
    });

    const req = httpMock.expectOne(
      `${environment.apiBaseUrl}/storefront/health`
    );

    expect(req.request.method).toBe('GET');

    req.flush(mockResponse);
  });
});
