# Member 02 — Seller, shop and catalog

A buyer opens **Sell on CraftBridge**, applies, and sees the review decision on the application page. Admin approval creates the shop and grants the seller role inside the same transaction. A rejected applicant sees the decision reason. An approved applicant can select **Manage my shop** to open the dashboard.

## Seller workflow

- Edit shop details, upload a profile and banner, and open the public artisan storefront by slug.
- Create a draft with a title, description, active category, USD price and opening stock. The price is stored in cents.
- Add product images while the draft is private, update visibility to published and save. Archive from the editor to remove the product from public discovery while preserving order history.
- Record stock adjustments with a reason. The inventory ledger shows available and reserved changes, the time and the actor. A published item with five or fewer available units counts as low stock.
- The dashboard links to a read-only paid orders view supplied by Member 04's shipping API. Member 04 owns fulfillment transitions.

## API handover

All paths are under /api/v1. The Angular client uses the shared authentication interceptor. Ownership comes from the authenticated user.

| Operation | Request | Result |
| --- | --- | --- |
| Apply and review | POST /seller/applications, GET /seller/application, POST /admin/seller-applications/{id}/decision | Application status and rejection reason, or approved shop |
| Dashboard and profile | GET /seller/dashboard, GET or PATCH /seller/shop | Dashboard metrics or shop details |
| Shop media | POST /seller/shop/images/profile or /seller/shop/images/banner | Updated shop |
| Create or list products | POST or GET /seller/products | Draft product or seller's list |
| Inspect, edit or archive | GET, PATCH or DELETE /seller/products/{id} | Seller-owned product |
| Product media | GET or POST /seller/products/{id}/images, DELETE /seller/product-images/{imageId} | Owner's images including drafts, upload or removal |
| Stock | GET /seller/products/{id}/inventory, POST /seller/products/{id}/inventory-adjustments, GET /seller/products/{id}/inventory-movements | Quantities and last 50 movements |
| Public catalog | GET /shops/{slug}, /categories, /products, /products/{id}, /products/{id}/images | Shop, categories and published products |

Example draft request, using an existing category ID:

```json
{"category_id":1,"title":"Clay bowl","description":"Handmade pottery","price_cents":1250,"stock_quantity":5}
```

Send images in multipart field `image`. Product uploads accept optional query parameter `alt_text`. Shop images allow up to 3 MB; product images allow up to 5 MB each and eight per product. The API stores files under CATALOG_UPLOAD_DIR (default uploads/catalog) and serves them at /media/catalog.

Member 04 passes its checkout transaction into ReserveForCheckout, ReleaseCheckoutReservation and ConfirmCheckoutReservation. Balance updates and ledger entries commit together.

## Local setup and tests

Use migrations in filename order and database/seeds/010_catalog_demo.sql on a fresh database. This workspace patch uses the existing schema. The project README and .env.example contain the application setup.

- From frontend, run `npm run build` and `npm test -- --watch=false --browsers=ChromeHeadless`.
- From backend, run `go test ./...`. To exercise the PostgreSQL reservation/release test, set M02_TEST_DB_URL to a migrated test database and run `go test ./internal/modules/catalog -run TestCheckoutInventoryLedger -count=1`. Its fixtures roll back.
- From the repository root, run `npx playwright test tests/e2e/seller-workspace.spec.js`. For all three tests, start the API on a disposable migrated database, seed categories and set M02_TEST_ADMIN_EMAIL and M02_TEST_ADMIN_PASSWORD to a test admin. M02_TEST_API defaults to http://localhost:8080/api/v1. These tests create their own applicants, products and media.
