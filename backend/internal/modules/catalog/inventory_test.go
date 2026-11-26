package catalog

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
	"testing"
)

// Exercise the transaction contract used by checkout. All fixtures roll back.
func TestCheckoutInventoryLedger(t *testing.T) {
	url := os.Getenv("M02_TEST_DB_URL")
	if url == "" {
		t.Skip("set M02_TEST_DB_URL to a migrated local test database")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	user := id("INSERT INTO users(email,password_hash) VALUES(gen_random_uuid()::text||'@example.test','fixture') RETURNING id")
	app := id("INSERT INTO seller_applications(user_id,store_name,country_code,status) VALUES($1,'Shop','LK','approved') RETURNING id", user)
	shop := id("INSERT INTO shops(owner_user_id,application_id,name,slug,country_code) VALUES($1,$2,'Shop',gen_random_uuid()::text,'LK') RETURNING id", user, app)
	category := id("INSERT INTO categories(name,slug) VALUES('Test',gen_random_uuid()::text) RETURNING id")
	product := id("INSERT INTO products(shop_id,category_id,title,description,price_cents,status) VALUES($1,$2,'Bowl','Clay',1250,'published') RETURNING id", shop, category)
	if _, err = tx.Exec(ctx, "INSERT INTO inventory(product_id,available_qty,reserved_qty) VALUES($1,3,0)", product); err != nil {
		t.Fatal(err)
	}
	svc := NewService(&Repository{})
	if err := svc.ReserveForCheckout(ctx, tx, product, 4); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("oversized reservation: %v", err)
	}
	if err := svc.ReserveForCheckout(ctx, tx, product, 3); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReleaseCheckoutReservation(ctx, tx, product, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmCheckoutReservation(ctx, tx, product, 2); err != nil {
		t.Fatal(err)
	}
	var available, reserved, count int
	err = tx.QueryRow(ctx, "SELECT available_qty,reserved_qty,(SELECT count(*) FROM inventory_movements WHERE product_id=$1) FROM inventory WHERE product_id=$1", product).Scan(&available, &reserved, &count)
	if err != nil {
		t.Fatal(err)
	}
	if available != 1 || reserved != 0 || count != 3 {
		t.Fatalf("available=%d reserved=%d movements=%d", available, reserved, count)
	}
}
