# Member 06 — Admin, Reporting & Integration Module

## Overview
Member 06 owns platform administration, financial & sales reporting (`verified payments - approved refunds`), append-only security audit logging, and core API/router integration across the CraftBridge platform.

---

## Database Schema & Seeds
- **Up Migration**: `database/migrations/050_admin_audit.up.sql`
  - Table: `audit_logs` (`id`, `actor_id`, `actor_name`, `action`, `target`, `outcome`, `request_id`, `details`, `created_at TIMESTAMPTZ`)
  - Index: `idx_audit_logs_created_at ON audit_logs (created_at DESC)`
- **Down Migration**: `database/migrations/050_admin_audit.down.sql`
- **Seed Data**: `database/seeds/050_admin_seed.sql`

---

## Backend API Endpoints (`/api/v1`)
All Member 06 endpoints are protected by `RequireAdminRole()` RBAC middleware.

### Admin Module (`/api/v1/admin`)
| Method | Endpoint | Description | Audit Action Recorded |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/admin/users` | List users with optional `role` and `status` filters | — |
| `PATCH` | `/api/v1/admin/users/:id/status` | Update user status (`ACTIVE`, `SUSPENDED`, `BANNED`) | `USER_STATUS_UPDATE` |
| `GET` | `/api/v1/admin/sellers/applications` | List seller applications by `status` | — |
| `PATCH` | `/api/v1/admin/sellers/:id/status` | Approve, reject, or suspend seller (`APPROVED`, `REJECTED`, `SUSPENDED`) | `SELLER_STATUS_UPDATE` |
| `PATCH` | `/api/v1/admin/refunds/:id/status` | Approve or reject refund (`APPROVED`, `REJECTED`, `PROCESSED`) | `REFUND_STATUS_UPDATE` |
| `GET` | `/api/v1/admin/categories` | List all catalog categories | — |
| `POST` | `/api/v1/admin/categories` | Create a new catalog category | `CATEGORY_CREATE` |
| `PUT` | `/api/v1/admin/categories/:id` | Update an existing category | `CATEGORY_UPDATE` |
| `DELETE` | `/api/v1/admin/categories/:id` | Delete a category by ID | `CATEGORY_DELETE` |

### Report Module (`/api/v1/reports`)
| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/reports/revenue` | Net revenue summary (`gross_verified_payments - total_refunds`) with optional `start_date` and `end_date` |
| `GET` | `/api/v1/reports/revenue/daily` | Daily time-series breakdown of verified payments, refunds, and net revenue |
| `GET` | `/api/v1/reports/sellers` | Per-seller order count, verified payments, refunds, and net revenue |

### Audit Module (`/api/v1/audit`)
| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/audit/logs` | Retrieve append-only audit logs ordered by `created_at DESC` |
| `POST` | `/api/v1/audit/logs` | Record a new immutable audit entry |

