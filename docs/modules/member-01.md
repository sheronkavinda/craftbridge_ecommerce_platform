# Member 01 — Identity, Access & Customer Accounts

**Owner:** Adeesha  
**Branch:** `feature/m01-auth`

## Scope

This feature owns customer registration and login, revocable authentication sessions, account profiles, delivery addresses, and shared authentication middleware. Public registration creates the `buyer` role only. Seller/admin role assignment remains controlled by the responsible team workflows.

## Database

Apply `database/migrations/001_identity.up.sql` before `010_sellers_catalog`. Roll back with `001_identity.down.sql`.

The migration creates users, roles, role assignments, sessions, password-reset tokens, and addresses. An address always belongs to its user; setting or deleting a default address preserves a single default when addresses remain.

## API

All paths are under `/api/v1`.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/auth/register` | Create a buyer account and sign in |
| `POST` | `/auth/login` | Verify credentials and issue a signed token |
| `POST` | `/auth/logout` | Revoke the current server-side session |
| `POST` | `/auth/forgot-password` | Request a password reset without account enumeration |
| `POST` | `/auth/reset-password` | Consume a single-use reset token |
| `GET` | `/auth/me` | Return the authenticated identity |
| `GET` / `PATCH` | `/me` | Read or update the authenticated user's profile |
| `GET` / `POST` | `/me/addresses` | List or create the authenticated user's addresses |
| `PATCH` / `DELETE` | `/me/addresses/:id` | Update or remove an owned address |

Address ownership comes from the authenticated token, never from a client-supplied user ID. The repository scopes every mutation by both address ID and authenticated user ID.

## Authentication integration

The current implementation uses an HS256 bearer JWT whose `jti` references a PostgreSQL session row. Protected middleware validates that row; logout revokes it, and password reset revokes the user's active sessions. JWT roles include all assigned roles so a buyer who later becomes a seller retains buyer access.

The Angular client stores the token in `localStorage`, sends only the `Authorization: Bearer` header, and clears local state / redirects on expired-session responses. The backend requires a valid Bearer JWT and active session by default. A legacy actor-header bypass exists only when both `APP_ENV=development` and `ENABLE_LOCAL_DEV_AUTH=1` are explicitly set; the sample environment does not enable it, and it must never be enabled outside isolated local testing.

The Member 01 integration decision is to keep bearer tokens in `localStorage` for compatibility with the shared API clients. This is not an HttpOnly-cookie/CSRF design; deployment owners should review the XSS risk and require HTTPS before production.

## Password-reset email

`forgot-password` stores only a SHA-256 token hash, expires it after 30 minutes, and sends a reset link through SMTP using STARTTLS. The message links to `FRONTEND_BASE_URL/reset-password?token=...`. The API returns the same generic message for known and unknown emails. If email delivery fails, the unused token is removed and the failure is logged without exposing account existence in the response.

Copy the root `.env.example` to `backend/.env` and set `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, and `SMTP_FROM` for your SMTP provider. `SMTP_HOST` is a hostname without a scheme; `SMTP_FROM` must be a sender address verified by the provider. Username and password must either both be set or both be empty. `FRONTEND_BASE_URL` defaults to `http://localhost:4200` for local development; non-local environments must use HTTPS. Never commit the populated `.env` file.

## Integration verification

The shared API accepts the Member 01 JWT/session claims on the other modules' authenticated routes. A live local check verified buyer access to account, wishlist, cart/orders, payments/refunds, and seller routes, with admin-only access correctly rejected. The database-backed auth integration test exercises reset-token generation, handoff to the email sender, hashed storage, failed-delivery cleanup, one-time consumption, session revocation, and password replacement:

```powershell
Set-Location backend
$env:RUN_POSTGRES_INTEGRATION = "1"
go test ./internal/modules/auth -run TestPostgresPasswordResetDeliveryAndTokenConsumption -count=1
Remove-Item Env:RUN_POSTGRES_INTEGRATION
```

The integration test requires PostgreSQL with all migrations applied and the connection settings configured in `backend/.env` or the environment.

## Validation

From the repository root:

```powershell
Set-Location backend
go test ./...
Set-Location ..\frontend
npm run build
npx ng test --watch=false --browsers=ChromeHeadless
Set-Location ..
npx playwright test tests/e2e/account.spec.ts
```

The account Playwright coverage exercises login, the protected account route, and address edits/removal using API stubs. The backend unit/handler tests cover buyer-only registration, JWT session claims, logout ownership, address validation, and cross-user address rejection.
