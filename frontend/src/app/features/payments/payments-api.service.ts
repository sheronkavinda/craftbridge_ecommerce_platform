import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpErrorResponse, HttpParams } from '@angular/common/http';
import { Observable, catchError, map, throwError } from 'rxjs';
import { environment } from '../../../environments/environment';
import {
  ApiEnvelope,
  BuyerPayment,
  InitiatePaymentResponse,
  PaginatedPayments,
  PaymentStatus,
} from './payments.models';

export class PaymentsApiError extends Error {
  constructor(
    public readonly status: number,
    override readonly message: string,
  ) {
    super(message);
    this.name = 'PaymentsApiError';
  }
}

@Injectable({
  providedIn: 'root',
})
export class PaymentsApiService {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl;

  initiatePayment(orderId: string): Observable<InitiatePaymentResponse> {
    const trimmedOrderId = encodeURIComponent(orderId.trim());
    return this.http
      .post<ApiEnvelope<InitiatePaymentResponse>>(
        `${this.baseUrl}/orders/${trimmedOrderId}/payments`,
        {},
      )
      .pipe(
        map((res) => res.data),
        catchError((err) => this.handleError(err)),
      );
  }

  getPayment(id: string): Observable<BuyerPayment> {
    const trimmedId = encodeURIComponent(id.trim());
    return this.http
      .get<ApiEnvelope<BuyerPayment>>(`${this.baseUrl}/me/payments/${trimmedId}`)
      .pipe(
        map((res) => res.data),
        catchError((err) => this.handleError(err)),
      );
  }

  listPayments(
    page = 1,
    pageSize = 20,
    status?: PaymentStatus,
  ): Observable<PaginatedPayments> {
    let params = new HttpParams()
      .set('page', String(page))
      .set('page_size', String(pageSize));
    if (status) {
      params = params.set('status', status);
    }

    return this.http
      .get<PaginatedPayments>(`${this.baseUrl}/me/payments`, { params })
      .pipe(
        map((res) => ({
          data: res.data ?? [],
          meta: res.meta ?? { page, page_size: pageSize, total: 0 },
        })),
        catchError((err) => this.handleError(err)),
      );
  }

  private handleError(error: unknown): Observable<never> {
    if (error instanceof HttpErrorResponse) {
      const backendMessage =
        typeof error.error === 'object' &&
        error.error !== null &&
        typeof error.error.error === 'string'
          ? error.error.error
          : error.message || 'Payment request failed';
      return throwError(() => new PaymentsApiError(error.status, backendMessage));
    }
    return throwError(
      () => new PaymentsApiError(0, error instanceof Error ? error.message : 'Unknown error'),
    );
  }
}
