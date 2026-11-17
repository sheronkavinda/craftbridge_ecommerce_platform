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

export interface ApiEnvelope<T> {
  data: T;
}
