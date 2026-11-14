export interface ApiResponse<T> {
  data: T;
  error?: string;
  message?: string;
}

export interface UserSummary {
  id: string;
  name: string;
  email: string;
  role: 'BUYER' | 'SELLER' | 'ADMIN' | string;
  status: 'ACTIVE' | 'SUSPENDED' | 'BANNED' | string;
  created_at: string;
}

export interface UpdateUserStatusPayload {
  status: 'ACTIVE' | 'SUSPENDED' | 'BANNED';
  reason?: string;
}

export interface SellerApplication {
  id: string;
  user_id: string;
  user_name?: string;
  user_email?: string;
  store_name: string;
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'SUSPENDED' | string;
  notes: string;
  submitted_at: string;
  reviewed_at?: string;
}

export interface UpdateSellerStatusPayload {
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'SUSPENDED';
  notes?: string;
}

export interface RefundRecord {
  id: string;
  order_id: string;
  amount: number;
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'PROCESSED' | 'COMPLETED' | string;
  reason: string;
  created_at: string;
  reviewed_at?: string;
}

export interface UpdateRefundStatusPayload {
  status: 'APPROVED' | 'REJECTED' | 'PROCESSED';
  reason?: string;
}

export interface Category {
  id: string;
  name: string;
  slug: string;
  description: string;
  is_active: boolean;
  created_at: string;
}

export interface CategoryPayload {
  name: string;
  slug?: string;
  description?: string;
  is_active?: boolean;
}

export interface RevenueSummary {
  gross_verified_payments: number;
  total_refunds: number;
  net_revenue: number;
  verified_payment_count: number;
  refund_count: number;
  start_date?: string;
  end_date?: string;
  generated_at: string;
}

export interface DailyRevenuePoint {
  date: string;
  verified_payments: number;
  refunds: number;
  net_revenue: number;
}

export interface SellerSalesReport {
  seller_id: string;
  store_name: string;
  order_count: number;
  verified_payments: number;
  refunds: number;
  net_revenue: number;
}

export interface AuditLog {
  id: number;
  actor_id: string;
  actor_name: string;
  action: string;
  target: string;
  outcome: 'SUCCESS' | 'FAILURE' | string;
  request_id: string;
  details: string;
  created_at: string;
}
