package order_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"craftbridge/backend/internal/modules/cart"
	"craftbridge/backend/internal/modules/catalog"
	"craftbridge/backend/internal/modules/order"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLocalCheckoutStock(t *testing.T) {
	if os.Getenv("RUN_M02_CHECKOUT") != "1" {
		t.Skip("requires the local CraftBridge demo database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(os.Getenv("DB_URL"))
	if err != nil {
		t.Fatal("could not read DB_URL")
	}
	if cfg.ConnConfig.Database != "craftbridge_dev" ||
		cfg.ConnConfig.Host != "host.docker.internal" {
		t.Fatal("expected craftbridge_dev on host.docker.internal")
	}

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const buyerID, addressID, productID = 1002, 1, 1

	checkStock := func(wantAvailable, wantReserved int) {
		t.Helper()
		var available, reserved int
		err := db.QueryRow(ctx, `
			SELECT available_qty, reserved_qty
			FROM inventory
			WHERE product_id = $1
		`, productID).Scan(&available, &reserved)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("available=%d reserved=%d", available, reserved)
		if available != wantAvailable || reserved != wantReserved {
			t.Fatalf("expected available=%d reserved=%d", wantAvailable, wantReserved)
		}
	}

	checkStock(8, 0)

	carts := cart.NewRepository(db)
	current, err := carts.GetCart(ctx, buyerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 0 {
		t.Fatal("demo buyer cart must be empty before running this test")
	}

	err = carts.AddItem(ctx, buyerID, cart.AddItemRequest{
		ProductID: productID,
		Quantity:  2,
	})
	if err != nil {
		t.Fatal(err)
	}

	orders := order.NewService(db, catalog.NewService(catalog.NewRepository(db)))
	key := fmt.Sprintf("m02-stock-%d", time.Now().UnixNano())
	created, err := orders.Checkout(ctx, buyerID, addressID, key)
	if err != nil {
		t.Errorf("checkout: %v", err)

		// Remove the two test mugs if checkout left them in the demo cart.
		after, readErr := carts.GetCart(ctx, buyerID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(after.Items) == 1 &&
			after.Items[0].ProductID == productID &&
			after.Items[0].Quantity == 2 {
			if removeErr := carts.RemoveItem(ctx, buyerID, after.Items[0].ID); removeErr != nil {
				t.Errorf("clean demo cart: %v", removeErr)
			}
		}
		return
	}

	t.Logf("order ID=%d", created.ID)

	// Cancel the demo order even if the reservation check fails.
	defer func() {
		if err := orders.CancelOrder(ctx, buyerID, created.ID); err != nil {
			t.Errorf("cancel order %d: %v", created.ID, err)
			return
		}
		checkStock(8, 0)
	}()

	checkStock(6, 2)
}