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

export interface ApiEnvelope<T> {
  data: T;
}
