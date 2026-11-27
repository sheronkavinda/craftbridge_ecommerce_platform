# Member 05 — Payments, Refunds & Receipts

**Owner**: Member 05 (Kaviya)  
**Branch**: `feature/m05-payments`

---

## 1. Overview & Scope

Member 05 owns the end-to-end financial lifecycle after an order is created at checkout:

1. **Payment Initiation & Hosted Checkout Redirect**: Creates or reuses an active `PENDING` payment attempt for an owned unpaid order and returns a server-signed PayHere Sandbox form payload (`action_url`, `method`, `fields`).
2. **Signed Provider Webhooks**: Receives PayHere `notify_url` callbacks at `POST /api/v1/webhooks/payments/:provider`, verifies the MD5 signature in constant time, deduplicates events by `(provider, provider_event_id)`, validates amount and currency against the payment attempt, transitions the attempt state machine, and notifies the order module on verified `SUCCESS`.
3. **Ambiguous Payment Reconciliation**: Background ticker and admin-triggered reconciliation (`POST /api/v1/admin/payments/reconcile`) that queries the PayHere Retrieval API for aged `PENDING` attempts (`SELECT ... FOR UPDATE SKIP LOCKED`) and expires stale attempts past the TTL.
4. **Buyer Payment History**: Paginated list (`GET /api/v1/me/payments`) and single attempt detail (`GET /api/v1/me/payments/:id`) scoped to the authenticated buyer.
5. **Buyer Refund Requests & Provider Refund Processing**: Buyers submit refund requests (`POST /api/v1/orders/:id/refund-requests`) for their own paid orders within the refund window, with `Idempotency-Key` support, open-request deduplication, and remaining refundable balance validation. Admins approve and execute provider refunds via `POST /api/v1/admin/refunds/:id/approve`. Buyers track requests and 4-step timelines via `GET /api/v1/me/refunds` and `GET /api/v1/me/refunds/:id`.
6. **Owner-Restricted Receipts**: Lazy, idempotent receipt generation for paid orders (`GET /api/v1/me/orders/:id/receipt`) and printable self-contained HTML download (`GET /api/v1/me/orders/:id/receipt/download`).
7. **Seller Earnings Summary**: Calculated projection (`GET /api/v1/seller/earnings`) derived strictly from verified (`SUCCESS`) payment attempts minus `COMPLETED` refunds, split into `Pending` (within hold window), `Available` (ready for payout), and `Paid` (from platform settlement records).
8. **Frontend Pages (`frontend/src/app/features/payments/`)**:
   - `/payments/redirect/:orderId` — Hosted PayHere redirect & hidden auto/manual POST form (`PaymentRedirectComponent`)
   - `/payments/result` — Live-polling payment status confirmation page matching Mockup 13 (`PaymentResultComponent`)
   - `/payments/history` — Buyer payment history with status filters, pagination, and actions (`PaymentHistoryComponent`)
   - `/payments/receipt/:orderId` — Printable receipt view with client-side HTML download and print (`ReceiptComponent`)
   - `/payments/refunds` — Buyer returns & refunds request form and 4-step request timeline matching Mockup 15 (`RefundRequestComponent`)
   - `/payments/seller-earnings` — Seller earnings summary, per-order breakdown, recent transactions, and payout history matching Mockup 23 (`SellerEarningsComponent`)

---

## 2. Architecture

### Backend Module Layout

All backend code lives under `backend/internal/modules/` and follows the `handler -> service -> repository` layered pattern with small local interfaces (`RepositoryStore`, `OrderReader`, `OrderNotifier`, `PaymentProvider`, `Refunder`) so every service and handler is unit-tested with in-memory fakes without requiring a live database or network connection:

- **`backend/internal/modules/payment/`**
  - `model.go` — Domain structs (`PaymentAttempt`, `PaymentEvent`, `OrderSummary`, `CheckoutPayload`, `ReconcileSummary`, `SellerEarningsDTO`, `SellerTransactionDTO`, `SellerPayoutDTO`) and sentinel errors.
  - `state.go` — Explicit payment attempt state machine (`TransitionAttemptStatus`).
  - `provider.go` — `PaymentProvider` interface (`Name`, `BuildCheckoutForm`, `VerifyWebhook`, `QueryPaymentStatus`, `IsConfigured`) and `Refunder` interface (`ProcessRefund`).
  - `provider_sandbox.go` — `PayHereSandboxProvider` implementation (checkout MD5 hash generation, constant-time `md5sig` webhook verification, OAuth2 + Retrieval API status check, and OAuth2 + Refund API call).
  - `order_notifier.go` — `OrderNotifier` interface and default `LogOrderNotifier` fallback.
  - `repository.go` — `RepositoryStore` interface and PostgreSQL (`pgxpool`) implementation (`CreateOrReusePendingAttempt`, `RecordWebhookEventTx`, `ClaimStalePendingAttempts`, `GetSellerEarnings`).
  - `service.go` — Payment initiation, webhook processing, reconciliation (`ReconcileStaleAttempts`), buyer history, and seller earnings calculation.
  - `earnings.go` — Pure helper `ComputeCommissionCents` (`round-half-up(merchandise_cents * percent / 100)`).
  - `reconciler.go` — Background periodic reconciliation worker (`StartReconcileLoop`).
  - `handler.go` — Gin HTTP handlers for payment initiation, webhooks, admin reconcile, buyer history, and seller earnings.
- **`backend/internal/modules/refund/`**
  - `model.go` — `Refund`, `PaidOrderContext`, `TimelineEntry`, `RefundDetailResponse`, `CreateRefundInput`, and sentinel errors.
  - `notifier.go` — `RefundNotifier` interface and `LogRefundNotifier`.
  - `repository.go` — `RepositoryStore` interface and PostgreSQL implementation (writes both Member 05 `amount_cents`/`decided_at` and Member 06 `amount`/`reviewed_at` columns).
  - `service.go` — Buyer refund eligibility, remaining balance calculation, idempotency, admin approval + provider refund call, and 4-step timeline builder.
  - `handler.go` — Gin HTTP handlers for buyer refund requests, buyer refund list/detail, and admin provider approval.
- **`backend/internal/modules/receipt/`**
  - `model.go` — `Receipt`, `ReceiptLineItem`, `ReceiptView`, `PaidOrderSnapshot`, and deterministic `FormatReceiptNumber` (`RCP-YYYYMMDD-<6 uppercase hex chars from md5(order_id)>`).
  - `html.go` — XSS-safe (`html/template`) printable receipt HTML renderer (`RenderReceiptHTML`).
  - `repository.go` — `RepositoryStore` interface and PostgreSQL implementation with `INSERT ... ON CONFLICT (order_id) DO NOTHING` lazy issuance.
  - `service.go` — Ownership check, paid status check (`409 Conflict` if unpaid), lazy receipt creation, and HTML generation.
  - `handler.go` — Gin HTTP handlers for JSON receipt view and `.html` file download.

### Adding Another Payment Gateway (e.g., Stripe)

Payment gateway logic is isolated behind `payment.PaymentProvider` and `payment.Refunder` in `backend/internal/modules/payment/provider.go`:

1. Create a new adapter (e.g., `stripe_provider.go`) implementing `PaymentProvider` (`Name() string`, `BuildCheckoutForm(...)`, `VerifyWebhook(...)`, `QueryPaymentStatus(...)`, `IsConfigured() bool`) and `Refunder` (`ProcessRefund(...)`).
2. Register or inject the provider into `payment.NewService` and `refund.NewService` in `backend/cmd/api/main.go`.
3. No changes are required in handlers, repositories, or database tables (`payment_attempts.provider` and `payment_events.provider` store the provider slug).

---

## 3. Environment Variables

Loaded in `backend/internal/platform/config/config.go` (`config.Load()`):

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PAYHERE_MERCHANT_ID` | `""` | PayHere Sandbox Merchant ID (required for checkout form signing & webhook verification) |
| `PAYHERE_MERCHANT_SECRET` | `""` | PayHere Sandbox Merchant Secret (fallback alias `PAYHERE_SECRET` is also checked in `cmd/api/main.go`) |
| `PAYHERE_CHECKOUT_URL` | `https://sandbox.payhere.lk/pay/checkout` | Hosted PayHere Sandbox checkout POST endpoint |
| `PAYHERE_APP_ID` | `""` | PayHere OAuth2 App ID for Retrieval & Refund REST APIs (optional; required for live API calls) |
| `PAYHERE_APP_SECRET` | `""` | PayHere OAuth2 App Secret for Retrieval & Refund REST APIs (optional; required for live API calls) |
| `PAYHERE_API_BASE_URL` | `https://sandbox.payhere.lk` | Base URL for PayHere OAuth2, Retrieval (`/merchant/v1/payment/search`), and Refund (`/merchant/v1/payment/refund`) APIs |
| `PAYMENT_RETURN_URL` | `http://localhost:4200/payments/result` | Browser return URL passed to PayHere checkout (`config.go` default is `""`, `cmd/api/main.go` falls back to `http://localhost:4200/payments/result`) |
| `PAYMENT_CANCEL_URL` | `http://localhost:4200/payments/result?cancelled=1` | Browser cancel URL passed to PayHere checkout (`config.go` default is `""`, `cmd/api/main.go` falls back to `http://localhost:4200/payments/result?cancelled=1`) |
| `PAYMENT_NOTIFY_URL` | `http://localhost:8080/api/v1/webhooks/payments/payhere` | Server-to-server webhook callback URL sent to PayHere (`config.go` default is `""`, `cmd/api/main.go` falls back to `http://localhost:8080/api/v1/webhooks/payments/payhere`) |
| `PAYMENT_RECONCILE_AFTER_MINUTES` | `30` | Minimum age (in minutes) before a `PENDING` payment attempt is eligible for provider status reconciliation |
| `PAYMENT_ATTEMPT_TTL_HOURS` | `24` | Maximum age (in hours) after which an unresolved `PENDING` attempt is marked `EXPIRED` |
| `PAYMENT_RECONCILE_INTERVAL_SECONDS` | `300` | Background reconciliation ticker interval in seconds (`0` disables the background loop) |
| `REFUND_WINDOW_DAYS` | `30` | Number of days after payment completion during which a buyer can request a refund |
| `PLATFORM_COMMISSION_PERCENT` | `10` | Platform commission percentage deducted from seller merchandise gross (`0`–`100`) |
| `PAYOUT_HOLD_DAYS` | `7` | Days after payment completion before a seller's net order earnings move from `Pending` to `Available` |

---

## 4. API Endpoints

All routes are mounted under `/api/v1` in `backend/cmd/api/main.go`. Responses use `{"data": ...}` on success and `{"error": "..."}` on failure.

| Method | Path | Auth Role | Purpose |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/orders/:id/payments` | `BUYER` | Initiate or reuse a `PENDING` payment attempt for an owned unpaid order; returns PayHere checkout form fields |
| `POST` | `/api/v1/webhooks/payments/:provider` | `PUBLIC` (MD5 signature verified) | Receive PayHere server-to-server payment notifications (`application/x-www-form-urlencoded` or JSON) |
| `POST` | `/api/v1/admin/payments/reconcile` | `ADMIN` | Trigger immediate reconciliation of aged `PENDING` payment attempts via provider status check |
| `GET` | `/api/v1/me/payments` | `BUYER` | List the authenticated buyer's payment attempts (`?page=1&limit=10&status=`) |
| `GET` | `/api/v1/me/payments/:id` | `BUYER` | Get details of a single payment attempt owned by the authenticated buyer |
| `POST` | `/api/v1/orders/:id/refund-requests` | `BUYER` | Submit a refund request for an owned paid order (supports `Idempotency-Key` header) |
| `POST` | `/api/v1/admin/refunds/:id/approve` | `ADMIN` | Approve a `PENDING` (or retry an `APPROVED`) refund and execute the provider refund call |
| `GET` | `/api/v1/me/refunds` | `BUYER` | List the authenticated buyer's refund requests (`?page=1&limit=10`) |
| `GET` | `/api/v1/me/refunds/:id` | `BUYER` | Get a single refund request with its 4-step `timeline` array |
| `GET` | `/api/v1/me/orders/:id/receipt` | `BUYER` | Lazily issue (if needed) and return the JSON receipt view for an owned paid order |
| `GET` | `/api/v1/me/orders/:id/receipt/download` | `BUYER` | Download a printable self-contained HTML receipt (`Content-Disposition: attachment; filename="receipt-<number>.html"`) |
| `GET` | `/api/v1/seller/earnings` | `SELLER` | Compute seller earnings summary (`pending_cents`, `available_cents`, `paid_cents`), sample breakdown, recent transactions, and payout history |

---

## 5. Database Schema

Defined in `database/migrations/040_payments_refunds.up.sql` (and reversible in `040_payments_refunds.down.sql`). Cross-module foreign IDs (`order_id`, `user_id`, `seller_id`) are stored as `TEXT` so the migration runs cleanly in any migration order.

### Tables & Relationships

1. **`payment_attempts`**
   - Primary key: `id TEXT`
   - Columns: `order_id TEXT NOT NULL`, `user_id TEXT NOT NULL`, `provider TEXT NOT NULL DEFAULT 'payhere_sandbox'`, `provider_reference TEXT`, `amount_cents BIGINT NOT NULL`, `currency TEXT NOT NULL DEFAULT 'USD'`, `status TEXT NOT NULL DEFAULT 'PENDING'` (`PENDING`, `SUCCESS`, `FAILED`, `EXPIRED`, `CANCELLED`), `failure_reason TEXT`, `created_at`, `updated_at`, `completed_at`.
   - Indexes: `idx_payment_attempts_order_id`, `idx_payment_attempts_user_id`, partial unique index `uq_payment_attempts_provider_ref` on `(provider, provider_reference) WHERE provider_reference IS NOT NULL`.
2. **`payment_events`**
   - Primary key: `id TEXT`
   - Columns: `provider TEXT NOT NULL`, `provider_event_id TEXT NOT NULL`, `attempt_id TEXT REFERENCES payment_attempts(id) ON DELETE SET NULL`, `event_type TEXT NOT NULL`, `payload JSONB`, `signature_valid BOOLEAN`, `processed_at`, `created_at`.
   - Unique constraint: `uq_payment_events_provider_event UNIQUE (provider, provider_event_id)` for webhook idempotency.
3. **`refunds`**
   - Primary key: `id TEXT`
   - Columns: `order_id TEXT NOT NULL`, `payment_attempt_id TEXT REFERENCES payment_attempts(id) ON DELETE SET NULL`, `requested_by TEXT`, `seller_id TEXT`, `reason TEXT`, `details TEXT`, `amount NUMERIC(12,2)`, `amount_cents BIGINT`, `currency TEXT DEFAULT 'USD'`, `status TEXT` (`PENDING`, `APPROVED`, `PROCESSED`, `COMPLETED`, `REJECTED`), `provider_refund_reference TEXT`, `idempotency_key TEXT UNIQUE`, `requested_at`, `reviewed_at`, `decided_at`, `decided_by`, `completed_at`, `created_at`, `updated_at`.
   - **Member 06 Compatibility**: Member 06's `admin` and `report` modules query `refunds.amount` (`NUMERIC(12,2)`), `refunds.reviewed_at`, and `status IN ('APPROVED','PROCESSED','COMPLETED')`. Member 05 writes both `amount_cents` (integer cents) and `amount = amount_cents / 100.0`, and populates both `decided_at` and `reviewed_at` when an admin approves a refund.
4. **`receipts`**
   - Primary key: `id TEXT`
   - Columns: `order_id TEXT NOT NULL UNIQUE`, `payment_attempt_id TEXT NOT NULL REFERENCES payment_attempts(id) ON DELETE RESTRICT`, `user_id TEXT NOT NULL`, `receipt_number TEXT NOT NULL UNIQUE`, `amount_cents BIGINT NOT NULL`, `currency TEXT NOT NULL DEFAULT 'USD'`, `issued_at`, `created_at`.

---

## 6. Core Business Rules & State Machines

### Payment Attempt State Machine (`payment/state.go`)
- Valid states: `PENDING`, `SUCCESS`, `FAILED`, `EXPIRED`, `CANCELLED`.
- Allowed transitions:
  - `PENDING -> SUCCESS | FAILED | EXPIRED | CANCELLED`
  - Idempotent self-transitions (`X -> X`) are no-ops.
  - **`SUCCESS` is strictly terminal**: once an attempt reaches `SUCCESS`, any later `FAILED`, `CANCELLED`, or `EXPIRED` webhook or reconciliation result is rejected (`ErrInvalidStateTransition`), preventing downgrade of a confirmed payment.
  - When an attempt transitions to `SUCCESS`, `failure_reason` is explicitly cleared (`NULL`) so an earlier mismatch or transient error does not linger.

### Webhook Verification & Idempotency (`payment/provider_sandbox.go`, `payment/service.go`)
- **Never trust browser redirects**: `/payments/result` only polls `GET /api/v1/me/payments/:id`. Only signed webhooks or server-side provider queries mutate payment status.
- **PayHere MD5 Signature Formula**:
  - Checkout `hash`: `UPPER(MD5(merchant_id + order_id + amount_formatted_2dp + currency + UPPER(MD5(merchant_secret))))`
  - Webhook `md5sig`: `UPPER(MD5(merchant_id + order_id + payhere_amount + payhere_currency + status_code + UPPER(MD5(merchant_secret))))`
  - Verified using `crypto/subtle.ConstantTimeCompare`. Invalid signatures return `400 Bad Request` before touching the database.
- **Status Code Mapping**:
  - `2` -> `SUCCESS`
  - `0` -> `PENDING`
  - `-1` -> `CANCELLED`
  - `-2` -> `FAILED`
  - `-3` -> `FAILED` (`failure_reason = "chargedback"`)
- **Deduplication**: Deterministic `provider_event_id` (from `payment_id + status_code + md5sig`, or SHA-256 of raw body). Duplicate deliveries hit `ON CONFLICT (provider, provider_event_id) DO NOTHING` and return `200 OK` with `"duplicate": true` without re-running transitions or re-notifying orders.
- **Amount & Currency Integrity**: If a webhook's `payhere_amount` or `payhere_currency` does not match the stored `payment_attempts` row, the event is logged with `failure_reason = "amount_mismatch"` (or `"currency_mismatch"`) and the attempt is **never** marked `SUCCESS`.

### Retry & Reconciliation Rules (`payment/service.go`, `payment/reconciler.go`)
- Initiating payment on an order that already has a fresh `PENDING` attempt reuses that attempt instead of creating duplicates. If the existing `PENDING` attempt is older than `PAYMENT_ATTEMPT_TTL_HOURS`, it is marked `EXPIRED` and a new attempt is created. If the order already has a `SUCCESS` attempt, initiation returns `409 Conflict`.
- `ReconcileStaleAttempts` claims up to 50 `PENDING` attempts older than `PAYMENT_RECONCILE_AFTER_MINUTES` using `SELECT ... FOR UPDATE SKIP LOCKED`:
  - Queries `provider.QueryPaymentStatus`.
  - If the provider confirms `SUCCESS`, transitions to `SUCCESS` and calls `OrderNotifier.MarkOrderPaid`.
  - If the provider confirms `FAILED` or `CANCELLED`, transitions accordingly.
  - If the provider still reports `PENDING` (or is unreachable) and the attempt age exceeds `PAYMENT_ATTEMPT_TTL_HOURS`, transitions the attempt to `EXPIRED` (`failure_reason = "reconciliation_timeout"`).

### Refund Lifecycle & Idempotency (`refund/service.go`)
- **Eligibility**: Caller must own the order; order must have a `SUCCESS` payment attempt within `REFUND_WINDOW_DAYS` (default 30 days); reason must be one of `arrived_damaged`, `wrong_item`, `missing_parts`, `not_as_described`, `late_delivery`, `other`; details must be `10..500` characters.
- **Partial & Full Refunds**: `amount_cents` defaults to the remaining refundable balance (`paid_amount_cents - sum(non_rejected_refunds)`). Requests exceeding the remaining refundable balance return `409 Conflict`.
- **3-Layer Idempotency**:
  1. If `Idempotency-Key` header matches an existing refund for the same buyer + order, returns the existing refund (`200 OK`). If used for a different order/buyer, returns `409 Conflict`.
  2. If the order already has an open (`PENDING`) refund request without a matching idempotency key, returns `409 Conflict` (`ErrRefundAlreadyOpen`).
  3. During admin approval (`POST /api/v1/admin/refunds/:id/approve`), `refund.ID` is passed to the provider as the provider idempotency key. Already-`COMPLETED` refunds return immediately without calling the provider again. Only a confirmed provider response marks the refund `COMPLETED`.

### Receipt Numbering (`receipt/model.go`)
- Receipts are issued lazily on the first `GET /api/v1/me/orders/:id/receipt` or `/download` request after an order has a `SUCCESS` payment attempt.
- Receipt number format: `RCP-YYYYMMDD-<6 uppercase hex chars from md5(order_id)>` (e.g., `RCP-20260405-591291`), guaranteed idempotent via `UNIQUE (order_id)` and `INSERT ... ON CONFLICT (order_id) DO NOTHING`.

### Seller Earnings Formula (`payment/earnings.go`, `payment/repository.go`)
- Only orders with a verified `payment_attempts.status = 'SUCCESS'` are included. Unpaid, `PENDING`, `FAILED`, `EXPIRED`, or `CANCELLED` attempts are never counted as income.
- Per-order calculation:
  - `commission_cents = (merchandise_cents * PLATFORM_COMMISSION_PERCENT + 50) / 100` (integer round-half-up)
  - `net_cents = max(0, merchandise_cents + shipping_cents - commission_cents - completed_refund_cents)`
- Bucket classification:
  - **`Pending`**: Net earnings from verified payments completed within the last `PAYOUT_HOLD_DAYS` (default 7 days), minus `paid_cents` not yet absorbed by older available orders.
  - **`Available`**: Net earnings from verified payments completed at least `PAYOUT_HOLD_DAYS` ago, minus `paid_cents` from `seller_payouts`.
  - **`Paid`**: Sum of settled payouts from Member 06's `seller_payouts` table (gracefully defaults to `0` and an empty `payouts: []` slice if `seller_payouts` does not exist yet, checked via `to_regclass('public.seller_payouts')`).

---

## 7. PayHere Sandbox Setup & Local Manual Testing

### Configuring PayHere Sandbox
1. Log in to your [PayHere Sandbox Merchant Portal](https://sandbox.payhere.lk) and copy:
   - **Merchant ID** -> set `PAYHERE_MERCHANT_ID` in `backend/.env`
   - **Merchant Secret** (for `localhost` domain) -> set `PAYHERE_MERCHANT_SECRET` in `backend/.env`
   - (Optional, for live Retrieval/Refund APIs) **App ID** and **App Secret** -> set `PAYHERE_APP_ID` and `PAYHERE_APP_SECRET`
2. To receive live webhooks from PayHere Sandbox on your local machine, expose port `8080` with a tunnel (e.g., `ngrok http 8080`) and set:
   ```env
   PAYMENT_NOTIFY_URL=https://<your-ngrok-subdomain>.ngrok-free.app/api/v1/webhooks/payments/payhere
   ```

### Simulating a Signed PayHere Webhook Locally

You do not need `ngrok` or a live PayHere account to test webhook verification locally.

1. **Log in** with `POST /api/v1/auth/login` and use the returned `access_token` as the `Authorization: Bearer <token>` header.
2. **Create the order and initiate payment**:
   - Add an item to cart (`POST /api/v1/me/cart/items`), then check out (`POST /api/v1/checkout` with an `Idempotency-Key` header and `{"address_id": <id>}`) to get `<order_id>`.
   - Initiate payment via `POST /api/v1/orders/<order_id>/payments`. The response returns `data.attempt_id` (e.g., `att-8881c4f13dbce139`) and `data.checkout.fields.amount` (e.g., `"32.00"`).
3. **Compute `md5sig`**: the PayHere webhook `order_id` field is the payment **attempt** reference (`<attempt_id>`), **not** the CraftBridge order ID. Compute `md5sig` as:

$$\text{md5sig} = \text{UPPER}\Big(\text{MD5}\big(\text{merchant\_id} + \text{attempt\_id} + \text{payhere\_amount} + \text{payhere\_currency} + \text{status\_code} + \text{UPPER}(\text{MD5}(\text{merchant\_secret}))\big)\Big)$$

4. **Post the signed webhook** to `POST /api/v1/webhooks/payments/payhere` (`application/x-www-form-urlencoded`) with `merchant_id`, `order_id=<attempt_id>`, `payment_id`, `payhere_amount` (2 decimals, e.g., `"32.00"`), `payhere_currency`, `status_code=2`, and `md5sig`.
5. **Verify the result**: `GET /api/v1/me/orders/<order_id>` shows `status: "processing"` and `payment_status: "paid"`, and `GET /api/v1/me/payments` shows `status: "SUCCESS"`.

**PowerShell example:**

```powershell
# 1. Log in as the buyer
$login = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/auth/login" `
  -ContentType "application/json" `
  -Body '{"email":"buyer@example.com","password":"Password123!"}'
$headers = @{ Authorization = "Bearer $($login.access_token)" }

# 2. Add to cart, checkout, and initiate payment
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/me/cart/items" `
  -Headers $headers -ContentType "application/json" `
  -Body '{"product_id":1,"quantity":1}' | Out-Null

$checkoutHeaders = @{
  Authorization     = "Bearer $($login.access_token)"
  "Idempotency-Key" = "idem-checkout-001"
}
$orderResp = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/checkout" `
  -Headers $checkoutHeaders -ContentType "application/json" `
  -Body '{"address_id":1}'
$orderId = $orderResp.id

$payResp = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/orders/$orderId/payments" `
  -Headers $headers -ContentType "application/json"
$attemptId = $payResp.data.attempt_id
$amount    = $payResp.data.checkout.fields.amount   # e.g. "32.00"
$currency  = $payResp.data.checkout.fields.currency # "USD"

# 3. Compute md5sig using attempt_id as PayHere's order_id
function Get-UpperMd5([string]$text) {
  $md5 = [System.Security.Cryptography.MD5]::Create()
  $bytes = [System.Text.Encoding]::UTF8.GetBytes($text)
  return ([System.BitConverter]::ToString($md5.ComputeHash($bytes)) -replace "-", "").ToUpperInvariant()
}

$merchantId     = "1211149"
$merchantSecret = "test_secret"
$statusCode     = "2"
$secretHash     = Get-UpperMd5 $merchantSecret
$md5Sig         = Get-UpperMd5 "$merchantId$attemptId$amount$currency$statusCode$secretHash"

# 4. Post the signed webhook
$webhookBody = @{
  merchant_id      = $merchantId
  order_id         = $attemptId
  payment_id       = "PH-SANDBOX-9001"
  payhere_amount   = $amount
  payhere_currency = $currency
  status_code      = $statusCode
  md5sig           = $md5Sig
}
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/webhooks/payments/payhere" `
  -ContentType "application/x-www-form-urlencoded" `
  -Body $webhookBody

# 5. Verify order and payment status
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/api/v1/me/orders/$orderId" -Headers $headers
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/api/v1/me/payments" -Headers $headers
```

**Bash / `curl` example:**

```bash
# 1. Log in to obtain access_token
JWT=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"buyer@example.com","password":"Password123!"}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')

# 2. Add to cart, checkout, and initiate payment
curl -s -X POST http://localhost:8080/api/v1/me/cart/items \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"product_id":1,"quantity":1}' > /dev/null

ORDER_ID=$(curl -s -X POST http://localhost:8080/api/v1/checkout \
  -H "Authorization: Bearer $JWT" \
  -H "Idempotency-Key: idem-checkout-001" \
  -H "Content-Type: application/json" \
  -d '{"address_id":1}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')

# Save attempt_id (e.g. att-8881c4f13dbce139) and checkout.fields.amount (e.g. 32.00)
curl -s -X POST "http://localhost:8080/api/v1/orders/$ORDER_ID/payments" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json"

ATTEMPT_ID="<attempt_id>"
AMOUNT="32.00"

# 3. Compute md5sig (using attempt_id as PayHere's order_id)
MD5SIG=$(ATTEMPT_ID="$ATTEMPT_ID" AMOUNT="$AMOUNT" python3 -c '
import hashlib, os
m_id, att_id, amt, cur, code, secret = "1211149", os.environ["ATTEMPT_ID"], os.environ["AMOUNT"], "USD", "2", "test_secret"
sec_md5 = hashlib.md5(secret.encode()).hexdigest().upper()
raw = f"{m_id}{att_id}{amt}{cur}{code}{sec_md5}"
print(hashlib.md5(raw.encode()).hexdigest().upper())
')

# 4. Post the signed webhook to mark the attempt SUCCESS
curl -X POST http://localhost:8080/api/v1/webhooks/payments/payhere \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "merchant_id=1211149" \
  --data-urlencode "order_id=$ATTEMPT_ID" \
  --data-urlencode "payment_id=PH-SANDBOX-9001" \
  --data-urlencode "payhere_amount=$AMOUNT" \
  --data-urlencode "payhere_currency=USD" \
  --data-urlencode "status_code=2" \
  --data-urlencode "md5sig=$MD5SIG"

# 5. Verify order (status="processing", payment_status="paid") and payment (status="SUCCESS")
curl -s -H "Authorization: Bearer $JWT" "http://localhost:8080/api/v1/me/orders/$ORDER_ID"
curl -s -H "Authorization: Bearer $JWT" "http://localhost:8080/api/v1/me/payments"
```

---

## 8. Local Demo Seed Data

Demo data is created through the real application flow rather than manual SQL inserts:

1. Register a seller account and submit a seller application.
2. Approve the seller application as an admin at `/admin`.
3. Add a published product with inventory and a `seller_shipping_rates` row for that shop.
4. Register/use a buyer account and create a buyer shipping address, then follow the checkout and payment flow in Section 7.

> **Note**: Per repository rules, local seed/test SQL files must not be committed to the repository.

### Direct Frontend URLs for Local Testing (Task C)

When running `npm start` (`http://localhost:4200`), navigate directly to any of the Member 05 routes below (the dev auth interceptor automatically attaches the appropriate buyer or seller headers):

- **Buyer Payment Redirect**: `http://localhost:4200/payments/redirect/<order_id>`
- **Buyer Payment Status / Result**: `http://localhost:4200/payments/result?attemptId=<attempt_id>&orderId=<order_id>`
- **Buyer Payment History**: `http://localhost:4200/payments/history`
- **Buyer Receipt View & Download**: `http://localhost:4200/payments/receipt/<order_id>`
- **Buyer Returns & Refunds**: `http://localhost:4200/payments/refunds?orderId=<order_id>`
- **Seller Earnings & Payout History**: `http://localhost:4200/payments/seller-earnings`

---

## 9. Integration Notes for Other Members

- **Member 04 (Orders & Checkout)**:
  - **Checkout -> Payment Handoff**: After creating an order at checkout, navigate the buyer to `/payments/redirect/:orderId` (or call `POST /api/v1/orders/:id/payments`). `GetOrderForPayment` reads `orders` (`id::text`, `buyer_id::text`, `grand_total_cents`, `currency`, `status`, `payment_status`, `expires_at`) and `users` (`name`, `email`). `InitiatePayment` only permits orders with `status = 'pending_payment'`, `payment_status = 'pending'`, and `expires_at > NOW()`.
  - **Real Order Notifier (`OrderServiceNotifier`)**: Wired in `backend/cmd/api/main.go` via `paymentService.SetNotifier(payment.NewOrderServiceNotifier(dbPool, orderSvc, order.ErrInvalidTransition))` after `m04.Register(...)`. On `SUCCESS`, it opens a `pgx.Tx` and calls `order.Service.MarkPaidTx(ctx, tx, orderID)`, committing `orders.payment_status = 'paid'` and `orders.status = 'processing'`. Already-paid orders are idempotent no-ops; `order.ErrInvalidTransition` (cancelled/expired late payment) logs a `WARNING` and returns `nil` so the provider does not retry forever; real DB errors are returned so `payment_events.processed_at` remains `NULL` for idempotent retry via webhook/reconciliation. `FAILED`/`CANCELLED`/`EXPIRED` outcomes leave the order unchanged so payment retries remain possible.
  - **Multi-Seller Earnings & Refunds**: `ListSellerPaidOrders` queries one row per `seller_orders` belonging to the seller's `shops` (`shops.owner_user_id::text`) whose parent `orders` row has a verified `SUCCESS` payment attempt (`MerchandiseCents = seller_orders.subtotal_cents`, `ShippingCents = seller_orders.shipping_cents`, `ProductTitle` from `order_items.product_title_snapshot`). `OrderForPayment.SellerID` is populated with `shops.owner_user_id::text` only when the order has a single `seller_order`, and left empty (`""`) for multi-seller orders; `ListSellerPaidOrders` subtracts `COMPLETED` refunds only when `refunds.seller_id` matches the seller or when `refunds.seller_id` is empty on a single-seller order.
  - **Receipts**: `receiptOrderSummaryAdapter` in `backend/cmd/api/main.go` queries `orders` (`subtotal_cents`, `shipping_total_cents`) and `order_items` joined via `seller_orders` (`product_title_snapshot`, `quantity`, `unit_price_cents`, `line_total_cents`).
- **Member 06 (Admin & Reports)**:
  - **Payout Settlement Table (`seller_payouts`)**: Seller earnings (`GET /api/v1/seller/earnings`) reads settled payouts from `public.seller_payouts` if the table exists (`to_regclass('public.seller_payouts') IS NOT NULL`).
  - **Currency Formatting Alignment**: Member 05 stores and displays amounts in `USD` (`amount_cents` formatted via `MoneyPipe`), matching the Checkout, Payment Status, Buyer Refunds, and Seller Earnings mockups (Mockups 12, 13, 15, 23), while keeping `refunds.amount` (`NUMERIC(12,2)`) populated for Member 06's admin/report queries. Note that Member 06's admin refund view currently formats `refunds.amount` with `CurrencyLkrPipe`; the team should align on USD vs LKR display in the admin view during final integration.
- **Member 01 (Auth) & Member 03 (Storefront / Layouts)**:
  - **JWT Authentication**: Protected API routes require a valid Bearer JWT issued by the Member 01 auth endpoints. Development actor headers are rejected by default; a legacy bypass requires both `APP_ENV=development` and `ENABLE_LOCAL_DEV_AUTH=1` and must only be used for isolated local testing.
  - **Layout Shells**: `payments-shell`, `account-shell`, and `seller-shell` under `frontend/src/app/features/payments/shared/` are self-contained layouts matching Mockups 12, 13, 15, and 23 and can be swapped for shared workspace shells once merged.

---

## 10. Known Limitations & Unverified Items

1. **PayHere Live Retrieval & Refund API Payloads**: The OAuth2 token exchange (`/merchant/v1/oauth/token`), Payment Retrieval API (`/merchant/v1/payment/search`), and Refund API (`/merchant/v1/payment/refund`) in `provider_sandbox.go` are implemented tolerantly against PayHere's documented JSON structures and thoroughly unit-tested with `httptest.Server`, but have not been exercised against a live PayHere Sandbox merchant account with real `PAYHERE_APP_ID` / `PAYHERE_APP_SECRET` credentials.
2. **Evidence Photo Upload on Refund Form**: The "Upload evidence photos" control on `/payments/refunds` is rendered as a disabled placeholder ("Photo upload coming soon — not required in demo") because the platform does not have a blob/media upload service in scope.
3. **Refund Order Notifier**: `refund.NewLogRefundNotifier` logs refund completion events; if Member 04 adds an order-level refund hook later, it can be wired into `refundService.SetNotifier(...)` in `backend/cmd/api/main.go`.
4. **Live PostgreSQL Query Execution**: Unit tests cover all service, handler, state-machine, and `OrderServiceNotifier` logic with in-memory fakes; the PostgreSQL queries in `GetOrderForPayment`, `ResolveSellerID`, `ListSellerPaidOrders`, and `receiptOrderSummaryAdapter` compile cleanly and should be smoke-tested against a live PostgreSQL database with migrations `001`–`040` applied.
