import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable, map } from 'rxjs';
import { environment } from '../../../environments/environment';
import {
  ApiResponse,
  AuditLog,
  Category,
  CategoryPayload,
  DailyRevenuePoint,
  DashboardSummary,
  RefundRecord,
  RevenueSummary,
  SellerApplication,
  SellerSalesReport,
  UpdateRefundStatusPayload,
  UpdateSellerStatusPayload,
  UpdateUserRolePayload,
  UpdateUserStatusPayload,
  UserSummary,
} from './admin.models';

@Injectable({
  providedIn: 'root',
})
export class AdminService {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl;

  getDashboardSummary(): Observable<DashboardSummary> {
    return this.http
      .get<ApiResponse<DashboardSummary>>(`${this.baseUrl}/admin/dashboard`)
      .pipe(map((res) => res.data));
  }

  listUsers(role = '', status = '', q = '', limit = 50, offset = 0): Observable<UserSummary[]> {
    let params = new HttpParams().set('limit', limit).set('offset', offset);
    if (role) params = params.set('role', role);
    if (status) params = params.set('status', status);
    const trimmedQ = q.trim();
    if (trimmedQ) params = params.set('q', trimmedQ);

    return this.http
      .get<ApiResponse<UserSummary[]>>(`${this.baseUrl}/admin/users`, { params })
      .pipe(map((res) => res.data ?? []));
  }

  updateUserStatus(userId: string, payload: UpdateUserStatusPayload): Observable<UserSummary> {
    return this.http
      .patch<ApiResponse<UserSummary>>(`${this.baseUrl}/admin/users/${encodeURIComponent(userId)}/status`, payload)
      .pipe(map((res) => res.data));
  }

  updateUserRole(userId: string, payload: UpdateUserRolePayload): Observable<UserSummary> {
    return this.http
      .patch<ApiResponse<UserSummary>>(`${this.baseUrl}/admin/users/${encodeURIComponent(userId)}/roles`, payload)
      .pipe(map((res) => res.data));
  }

  listSellerApplications(status = '', limit = 50, offset = 0): Observable<SellerApplication[]> {
    let params = new HttpParams().set('limit', limit).set('offset', offset);
    if (status) params = params.set('status', status);

    return this.http
      .get<ApiResponse<SellerApplication[]>>(`${this.baseUrl}/admin/sellers/applications`, { params })
      .pipe(map((res) => res.data ?? []));
  }

  updateSellerStatus(
    sellerId: string,
    payload: UpdateSellerStatusPayload,
  ): Observable<Pick<SellerApplication, 'id' | 'store_name' | 'status' | 'notes'>> {
    const id = encodeURIComponent(sellerId);
    const decisionUrl = `${this.baseUrl}/admin/seller-applications/${id}/decision`;

    if (payload.status === 'APPROVED') {
      return this.http
        .post<{ shop: { name: string } }>(decisionUrl, { decision: 'approve' })
        .pipe(map(({ shop }) => ({
          id: sellerId,
          store_name: shop.name,
          status: 'APPROVED',
          notes: '',
        })));
    }

    if (payload.status === 'REJECTED') {
      return this.http
        .post<{
          application: { id: number; store_name: string; status: string; decision_reason: string | null };
        }>(decisionUrl, { decision: 'reject', reason: payload.notes?.trim() ?? '' })
        .pipe(map(({ application }) => ({
          id: String(application.id),
          store_name: application.store_name,
          status: application.status.toUpperCase(),
          notes: application.decision_reason ?? '',
        })));
    }

    return this.http
      .patch<ApiResponse<SellerApplication>>(`${this.baseUrl}/admin/sellers/${id}/status`, payload)
      .pipe(map((res) => res.data));
  }

  listRefunds(status = '', limit = 50, offset = 0): Observable<RefundRecord[]> {
    let params = new HttpParams().set('limit', limit).set('offset', offset);
    if (status) params = params.set('status', status);

    return this.http
      .get<ApiResponse<RefundRecord[]>>(`${this.baseUrl}/admin/refunds`, { params })
      .pipe(map((res) => res.data ?? []));
  }

  updateRefundStatus(refundId: string, payload: UpdateRefundStatusPayload): Observable<RefundRecord> {
    return this.http
      .patch<ApiResponse<RefundRecord>>(`${this.baseUrl}/admin/refunds/${encodeURIComponent(refundId)}/status`, payload)
      .pipe(map((res) => res.data));
  }

  listCategories(): Observable<Category[]> {
    return this.http
      .get<ApiResponse<Category[]>>(`${this.baseUrl}/admin/categories`)
      .pipe(map((res) => res.data ?? []));
  }

  createCategory(payload: CategoryPayload): Observable<Category> {
    return this.http
      .post<ApiResponse<Category>>(`${this.baseUrl}/admin/categories`, payload)
      .pipe(map((res) => res.data));
  }

  updateCategory(categoryId: string, payload: CategoryPayload): Observable<Category> {
    return this.http
      .put<ApiResponse<Category>>(`${this.baseUrl}/admin/categories/${encodeURIComponent(categoryId)}`, payload)
      .pipe(map((res) => res.data));
  }

  deleteCategory(categoryId: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/admin/categories/${encodeURIComponent(categoryId)}`);
  }

  getRevenueSummary(startDate = '', endDate = ''): Observable<RevenueSummary> {
    let params = new HttpParams();
    if (startDate) params = params.set('start_date', startDate);
    if (endDate) params = params.set('end_date', endDate);

    return this.http
      .get<ApiResponse<RevenueSummary>>(`${this.baseUrl}/reports/revenue`, { params })
      .pipe(map((res) => res.data));
  }

  getDailyRevenue(startDate = '', endDate = '', limit = 30): Observable<DailyRevenuePoint[]> {
    let params = new HttpParams().set('limit', limit);
    if (startDate) params = params.set('start_date', startDate);
    if (endDate) params = params.set('end_date', endDate);

    return this.http
      .get<ApiResponse<DailyRevenuePoint[]>>(`${this.baseUrl}/reports/revenue/daily`, { params })
      .pipe(map((res) => res.data ?? []));
  }

  getSellerRevenue(startDate = '', endDate = '', limit = 50): Observable<SellerSalesReport[]> {
    let params = new HttpParams().set('limit', limit);
    if (startDate) params = params.set('start_date', startDate);
    if (endDate) params = params.set('end_date', endDate);

    return this.http
      .get<ApiResponse<SellerSalesReport[]>>(`${this.baseUrl}/reports/sellers`, { params })
      .pipe(map((res) => res.data ?? []));
  }

  listAuditLogs(actorId = '', action = '', outcome = '', limit = 25, offset = 0): Observable<AuditLog[]> {
    let params = new HttpParams().set('limit', limit).set('offset', offset);
    if (actorId) params = params.set('actor_id', actorId);
    if (action) params = params.set('action', action);
    if (outcome) params = params.set('outcome', outcome);

    return this.http
      .get<ApiResponse<AuditLog[]>>(`${this.baseUrl}/admin/audit-logs`, { params })
      .pipe(map((res) => res.data ?? []));
  }
}
