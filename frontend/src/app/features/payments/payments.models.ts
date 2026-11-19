export type PaymentStatus = 'PENDING' | 'SUCCESS' | 'FAILED' | 'EXPIRED' | 'CANCELLED';

export interface BuyerPayment {
  id: string;
  order_id: string;
  order_number: string;
  order_status?: string;
  status: PaymentStatus;
  amount_cents: number;
  amount: string;
  currency: string;
  provider: string;
  provider_reference?: string;
  public_reason?: string;
  created_at: string;
  completed_at?: string;
}

export interface CheckoutDescriptor {
  checkout_url: string;
  method: string;
  fields: Record<string, string>;
}

export interface InitiatePaymentResponse {
  attempt_id: string;
  status: PaymentStatus;
  checkout: CheckoutDescriptor;
}

export interface PaginationMeta {
  page: number;
  page_size: number;
  total: number;
}

export interface PaginatedPayments {
  data: BuyerPayment[];
  meta: PaginationMeta;
}

export interface ReceiptItem {
  title: string;
  quantity: number;
  unit_price_cents: number;
  unit_price?: string;
  line_total_cents?: number;
  line_total?: string;
}

export interface ReceiptView {
  receipt_number: string;
  issued_at: string;
  order_id: string;
  order_number: string;
  buyer_name: string;
  buyer_email: string;
  payment_method: string;
  provider_reference: string;
  amount_cents: number;
  amount: string;
  currency: string;
  items: ReceiptItem[];
  refunded_cents: number;
  net_paid_cents: number;
  status: string;
}

export interface DownloadedReceipt {
  blob: Blob;
  filename: string;
}

export type RefundReason =
  | 'ARRIVED_DAMAGED'
  | 'NOT_AS_DESCRIBED'
  | 'WRONG_ITEM'
  | 'LATE_DELIVERY'
  | 'CHANGED_MIND'
  | 'OTHER';

export type RefundStatus =
  | 'PENDING'
  | 'APPROVED'
  | 'PROCESSED'
  | 'COMPLETED'
  | 'REJECTED';

export type RefundTimelineStepKey =
  | 'submitted'
  | 'under_review'
  | 'approved'
  | 'refund_issued';

export interface RefundTimelineStep {
  step: RefundTimelineStepKey;
  at?: string;
  done: boolean;
}

export interface BuyerRefund {
  id: string;
  order_id: string;
  order_number: string;
  status: RefundStatus;
  reason: RefundReason | string;
  details: string;
  amount_cents: number;
  amount: string;
  currency: string;
  requested_at: string;
  decided_at?: string;
  completed_at?: string;
  timeline?: RefundTimelineStep[];
}

export interface CreateRefundBody {
  reason: RefundReason;
  details: string;
  amount: string;
}

export interface CreateRefundResult {
  refund: BuyerRefund;
  replayed: boolean;
}

export interface PaginatedRefunds {
  data: BuyerRefund[];
  meta: PaginationMeta;
}

export interface SellerEarningsSummary {
  pending_cents: number;
  available_cents: number;
  paid_cents: number;
  currency: string;
}

export interface SellerTransaction {
  order_id: string;
  order_number: string;
  date: string;
  product_title: string;
  gross_cents: number;
  shipping_cents: number;
  commission_cents: number;
  refund_cents: number;
  adjustments_cents: number;
  net_cents: number;
  status: 'COMPLETED' | 'REFUNDED';
}

export interface SellerPayout {
  id?: string;
  payout_number?: string;
  initiated_at?: string;
  created_at?: string;
  method?: string;
  external_reference?: string;
  order_count?: number;
  amount_cents?: number;
  amount?: string;
  currency?: string;
  status?: string;
  settled_at?: string;
}

export interface SellerEarnings {
  summary: SellerEarningsSummary;
  transactions: SellerTransaction[];
  payouts: SellerPayout[];
}

export interface SellerEarningsResponse {
  data: SellerEarnings;
  meta: PaginationMeta;
}

export interface ApiEnvelope<T> {
  data: T;
}
