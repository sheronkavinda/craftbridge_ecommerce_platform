
package order

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{10,100}$`)

type Service struct {
	db        *pgxpool.Pool
	repo      *Repository
	inventory InventoryManager
}

func NewService(
	db *pgxpool.Pool,
	inventory InventoryManager,
) *Service {
	return &Service{
		db: db,
		repo: NewRepository(db),
		inventory: inventory,
	}
}

func (s *Service) ListOrders(
	ctx context.Context,
	buyerID int64,
) ([]OrderSummary, error) {
	if buyerID <= 0 {
		return nil, ErrInvalidOrder
	}
	return s.repo.ListBuyerOrders(ctx, buyerID)
}

func (s *Service) GetOrder(
	ctx context.Context,
	buyerID, orderID int64,
) (*Order, error) {
	if buyerID <= 0 || orderID <= 0 {
		return nil, ErrInvalidOrder
	}
	return s.repo.GetBuyerOrder(ctx, buyerID, orderID)
}

// Checkout creates a pending order and reserves inventory.
// No payment provider is called inside this transaction.
func (s *Service) Checkout(
	ctx context.Context,
	buyerID, addressID int64,
	idempotencyKey string,
) (*Order, error) {
	if buyerID <= 0 || addressID <= 0 ||
		!keyPattern.MatchString(idempotencyKey) {
		return nil, ErrInvalidOrder
	}

	if s.inventory == nil {
		return nil, ErrInventoryUnavailable
	}

	// Return an existing order for a repeated checkout key.
	existingID, existingAddress, err := s.repo.FindByKey(
		ctx, buyerID, idempotencyKey,
	)
	if err == nil {
		if existingAddress != addressID {
			return nil, ErrIdempotencyConflict
		}
		return s.repo.GetBuyerOrder(ctx, buyerID, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock this buyer's cart so cart changes cannot occur
	// during checkout.
	var cartID int64

	err = tx.QueryRow(ctx, `
		SELECT id FROM carts
		WHERE buyer_id = $1 FOR UPDATE
	`, buyerID).Scan(&cartID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmptyCart
	}
	if err != nil {
		return nil, err
	}

	// Check again after locking the cart.
	// Another request may have completed this key
	// while the current request waited for the lock.
	var repeatedOrderID, repeatedAddressID int64

	repeatedErr := tx.QueryRow(ctx, `
		SELECT id, address_id
		FROM orders
		WHERE buyer_id = $1 AND idempotency_key = $2
	`, buyerID, idempotencyKey).Scan(
		&repeatedOrderID, &repeatedAddressID,
	)

	if repeatedErr == nil {
		if repeatedAddressID != addressID {
			return nil, ErrIdempotencyConflict
		}

		if err := tx.Rollback(ctx); err != nil {
			return nil, err
		}

		return s.repo.GetBuyerOrder(
			ctx, buyerID, repeatedOrderID,
		)
	}

	if !errors.Is(repeatedErr, pgx.ErrNoRows) {
		return nil, repeatedErr
	}


	// Read the address from Adeesha's owned table.
	// Never trust an address supplied directly by the browser.
	var addressJSON, country string

	err = tx.QueryRow(ctx, `
		SELECT (to_jsonb(a) - 'user_id')::text,
		       UPPER(COALESCE(
		           to_jsonb(a)->>'country_code', ''
		       ))
		FROM addresses a
		WHERE a.id = $1 AND a.user_id = $2
	`, addressID, buyerID).Scan(
		&addressJSON, &country,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidAddress
	}
	if err != nil {
		return nil, err
	}

	if country != "US" {
		return nil, ErrInvalidAddress
	}

	// Lock inventory rows in product order.
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.shop_id, p.title, p.price_cents,
		       ci.quantity, i.available_qty,
		       (p.status = 'published'
		        AND s.status = 'active'
		        AND cat.is_active = TRUE)
		FROM cart_items ci
		JOIN products p ON p.id = ci.product_id
		JOIN shops s ON s.id = p.shop_id
		JOIN categories cat ON cat.id = p.category_id
		JOIN inventory i ON i.product_id = p.id
		WHERE ci.cart_id = $1
		ORDER BY p.id
		FOR UPDATE OF i
	`, cartID)
	if err != nil {
		return nil, err
	}

	groups := make(map[int64]*checkoutGroup)
	var subtotal int64
	itemCount := 0

	for rows.Next() {
		var item checkoutItem
		var available int32
		var eligible bool

		err := rows.Scan(
			&item.ProductID, &item.ShopID,
			&item.Title, &item.Price,
			&item.Quantity, &available,
			&eligible,
		)
		if err != nil {
			rows.Close()
			return nil, err
		}

		itemCount++

		if itemCount > 100 ||
			!eligible ||
			item.Quantity <= 0 ||
			item.Quantity > 99 ||
			available < item.Quantity ||
			item.Price <= 0 ||
			item.Price > 100000000 {
			rows.Close()
			return nil, ErrStockUnavailable
		}

		lineTotal := item.Price * int64(item.Quantity)
		subtotal += lineTotal

		group, exists := groups[item.ShopID]
		if !exists {
			group = &checkoutGroup{ShopID: item.ShopID}
			groups[item.ShopID] = group
		}

		group.Subtotal += lineTotal
		group.Items = append(group.Items, item)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if itemCount == 0 {
		return nil, ErrEmptyCart
	}

	// Initial shipping policy: server-configured zero fee.
	// Replace with approved shipping profiles in integration.
	const shippingTotal int64 = 0
	grandTotal := subtotal + shippingTotal

	var orderID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO orders (
			buyer_id, address_id, address_snapshot,
			idempotency_key, subtotal_cents,
			shipping_total_cents, grand_total_cents,
			currency, payment_status, status, expires_at
		)
		VALUES (
			$1, $2, $3::jsonb, $4, $5, $6, $7,
			'USD', 'pending', 'pending_payment',
			NOW() + INTERVAL '30 minutes'
		)
		ON CONFLICT (buyer_id, idempotency_key)
		DO NOTHING
		RETURNING id
	`, buyerID, addressID, addressJSON,
		idempotencyKey, subtotal,
		shippingTotal, grandTotal).Scan(&orderID)

	if errors.Is(err, pgx.ErrNoRows) {
		// Another request completed this checkout key first.
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return nil, rollbackErr
		}
		id, savedAddress, lookupErr := s.repo.FindByKey(
			ctx, buyerID, idempotencyKey,
		)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if savedAddress != addressID {
			return nil, ErrIdempotencyConflict
		}
		return s.repo.GetBuyerOrder(ctx, buyerID, id)
	}
	if err != nil {
		return nil, err
	}

	shopIDs := make([]int64, 0, len(groups))
	for id := range groups {
		shopIDs = append(shopIDs, id)
	}
	sort.Slice(shopIDs, func(i, j int) bool {
		return shopIDs[i] < shopIDs[j]
	})

	// One parent order, one suborder per seller.
	for _, shopID := range shopIDs {
		group := groups[shopID]

		var sellerOrderID int64

		err = tx.QueryRow(ctx, `
			INSERT INTO seller_orders (
				order_id, shop_id, subtotal_cents,
				shipping_cents, status
			)
			VALUES ($1, $2, $3, 0, 'pending_payment')
			RETURNING id
		`, orderID, shopID, group.Subtotal).Scan(
			&sellerOrderID,
		)
		if err != nil {
			return nil, err
		}

		for _, item := range group.Items {
			var orderItemID int64

			err = tx.QueryRow(ctx, `
				INSERT INTO order_items (
					seller_order_id, product_id,
					product_title_snapshot,
					unit_price_cents, quantity,
					line_total_cents
				)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id
			`, sellerOrderID, item.ProductID,
				item.Title, item.Price, item.Quantity,
				item.Price*int64(item.Quantity),
			).Scan(&orderItemID)

			if err != nil {
				return nil, err
			}

			// Kanishka's inventory implementation handles
			// the underlying row updates.
			err = s.inventory.ReserveForCheckout(
				ctx, tx, item.ProductID, item.Quantity,
			)
			if err != nil {
				return nil, ErrStockUnavailable
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO stock_reservations (
					order_item_id, product_id,
					quantity, status
				)
				VALUES ($1, $2, $3, 'reserved')
			`, orderItemID, item.ProductID,
				item.Quantity)

			if err != nil {
				return nil, err
			}
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO order_status_history (
			order_id, to_status, reason
		)
		VALUES ($1, 'pending_payment', 'checkout_created')
	`, orderID)
	if err != nil {
		return nil, err
	}

	// Prevent repeating this order with stale cart items.
	_, err = tx.Exec(ctx, `
		DELETE FROM cart_items WHERE cart_id = $1
	`, cartID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.repo.GetBuyerOrder(ctx, buyerID, orderID)
}

// Change reserved stock exactly once.
// Caller must already hold the parent order lock.
func (s *Service) transitionReservations(
	ctx context.Context,
	tx pgx.Tx,
	orderID int64,
	action string,
) error {
	rows, err := tx.Query(ctx, `
		SELECT sr.id, sr.product_id,
		       sr.quantity, sr.status
		FROM stock_reservations sr
		JOIN order_items oi ON oi.id = sr.order_item_id
		JOIN seller_orders so ON so.id = oi.seller_order_id
		WHERE so.order_id = $1
		ORDER BY sr.product_id, sr.id
		FOR UPDATE OF sr
	`, orderID)
	if err != nil {
		return err
	}

	type reservation struct {
		ID        int64
		ProductID int64
		Quantity  int32
		Status    string
	}

	items := make([]reservation, 0)

	for rows.Next() {
		var item reservation
		if err := rows.Scan(
			&item.ID, &item.ProductID,
			&item.Quantity, &item.Status,
		); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(items) == 0 {
		return ErrInvalidTransition
	}

	for _, item := range items {
		if item.Status != "reserved" {
			return ErrInvalidTransition
		}

		var next string

		switch action {
		case "commit":
			err = s.inventory.ConfirmCheckoutReservation(
				ctx, tx, item.ProductID, item.Quantity,
			)
			next = "committed"
		case "release":
			err = s.inventory.ReleaseCheckoutReservation(
				ctx, tx, item.ProductID, item.Quantity,
			)
			next = "released"
		default:
			return ErrInvalidTransition
		}

		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE stock_reservations
			SET status = $2, updated_at = NOW()
			WHERE id = $1 AND status = 'reserved'
		`, item.ID, next)
		if err != nil {
			return err
		}
	}

	return nil
}

// MarkPaidTx is NOT an HTTP endpoint.
// Only Kaviya's VERIFIED payment service may call it,
// inside the same database transaction as its payment event.
func (s *Service) MarkPaidTx(
	ctx context.Context,
	tx pgx.Tx,
	orderID int64,
) error {
	if tx == nil || orderID <= 0 || s.inventory == nil {
		return ErrInvalidOrder
	}

	var status, paymentStatus string
	var expiresAt time.Time

	err := tx.QueryRow(ctx, `
		SELECT status, payment_status, expires_at
		FROM orders
		WHERE id = $1 FOR UPDATE
	`, orderID).Scan(&status, &paymentStatus, &expiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}

	if paymentStatus == "paid" &&
		(status == "processing" || status == "completed") {
		return nil
	}

	if status != "pending_payment" ||
		paymentStatus != "pending" ||
		!time.Now().Before(expiresAt) {
		return ErrInvalidTransition
	}

	if err := s.transitionReservations(
		ctx, tx, orderID, "commit",
	); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET payment_status = 'paid',
		    status = 'processing',
		    updated_at = NOW()
		WHERE id = $1
	`, orderID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE seller_orders
		SET status = 'processing', updated_at = NOW()
		WHERE order_id = $1
	`, orderID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO order_status_history (
			order_id, from_status, to_status, reason
		)
		VALUES (
			$1, 'pending_payment', 'processing',
			'verified_payment'
		)
	`, orderID)

	return err
}

// CancelOrder releases stock for an unpaid order.
func (s *Service) CancelOrder(
	ctx context.Context,
	buyerID, orderID int64,
) error {
	return s.closePending(ctx, orderID, buyerID, "cancelled")
}

func (s *Service) closePending(
	ctx context.Context,
	orderID, buyerID int64,
	target string,
) error {
	if orderID <= 0 || s.inventory == nil {
		return ErrInvalidOrder
	}
	if target != "cancelled" && target != "expired" {
		return ErrInvalidOrder
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var ownerID int64
	var status, paymentStatus string
	var expiresAt time.Time

	err = tx.QueryRow(ctx, `
		SELECT buyer_id, status,
		       payment_status, expires_at
		FROM orders
		WHERE id = $1 FOR UPDATE
	`, orderID).Scan(
		&ownerID, &status, &paymentStatus, &expiresAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}

	if buyerID > 0 && ownerID != buyerID {
		return ErrOrderNotFound
	}

	if status == target {
		return nil
	}

	if status != "pending_payment" ||
		paymentStatus != "pending" {
		return ErrInvalidTransition
	}

	if target == "expired" && time.Now().Before(expiresAt) {
		return ErrInvalidTransition
	}

	if err := s.transitionReservations(
		ctx, tx, orderID, "release",
	); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE seller_orders
		SET status = 'cancelled', updated_at = NOW()
		WHERE order_id = $1
	`, orderID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, orderID, target)
	if err != nil {
		return err
	}

	var actor any
	if buyerID > 0 {
		actor = buyerID
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO order_status_history (
			order_id, from_status, to_status,
			actor_user_id, reason
		)
		VALUES (
			$1, 'pending_payment', $2, $3, $4
		)
	`, orderID, target, actor, target)

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
