package order

import (
	"context"
	"errors"
	"fmt"
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

func NewService(db *pgxpool.Pool, inventory InventoryManager) *Service {
	return &Service{
		db:        db,
		repo:      NewRepository(db),
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

// Checkout creates one parent order and seller-specific suborders.
// Product prices and shipping charges come from PostgreSQL.
func (s *Service) Checkout(
	ctx context.Context,
	buyerID, addressID int64,
	key string,
) (*Order, error) {
	if buyerID <= 0 || addressID <= 0 ||
		!keyPattern.MatchString(key) {
		return nil, ErrInvalidOrder
	}

	if s.inventory == nil {
		return nil, ErrInventoryUnavailable
	}

	// Return the original order for a repeated checkout key.
	previousID, previousAddress, err := s.repo.FindByKey(
		ctx, buyerID, key,
	)

	if err == nil {
		if previousAddress != addressID {
			return nil, ErrIdempotencyConflict
		}

		return s.repo.GetBuyerOrder(ctx, buyerID, previousID)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock the buyer's cart to serialize checkout.
	var cartID int64

	err = tx.QueryRow(ctx, `
		SELECT id
		FROM carts
		WHERE buyer_id = $1
		FOR UPDATE
	`, buyerID).Scan(&cartID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmptyCart
	}
	if err != nil {
		return nil, err
	}

	// Recheck idempotency after acquiring the cart lock.
	var repeatedID, repeatedAddress int64

	retryErr := tx.QueryRow(ctx, `
		SELECT id, address_id
		FROM orders
		WHERE buyer_id = $1
		  AND idempotency_key = $2
	`, buyerID, key).Scan(
		&repeatedID,
		&repeatedAddress,
	)

	if retryErr == nil {
		if repeatedAddress != addressID {
			return nil, ErrIdempotencyConflict
		}

		if err := tx.Rollback(ctx); err != nil {
			return nil, err
		}

		return s.repo.GetBuyerOrder(
			ctx, buyerID, repeatedID,
		)
	}

	if !errors.Is(retryErr, pgx.ErrNoRows) {
		return nil, retryErr
	}

	// Validate delivery address ownership.
	// Assumes addresses.user_id and addresses.country_code.
	var addressSnapshot, country string

	err = tx.QueryRow(ctx, `
		SELECT
			(to_jsonb(a) - 'user_id')::text,
			UPPER(
				COALESCE(to_jsonb(a)->>'country_code', '')
			)
		FROM addresses a
		WHERE a.id = $1
		  AND a.user_id = $2
	`, addressID, buyerID).Scan(
		&addressSnapshot,
		&country,
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

	// Read live product data and lock stock rows.
	rows, err := tx.Query(ctx, `
		SELECT
			p.id,
			p.shop_id,
			p.title,
			p.price_cents,
			ci.quantity,
			i.available_qty,
			(
				p.status = 'published'
				AND sh.status = 'active'
				AND cat.is_active = TRUE
			)
		FROM cart_items ci
		JOIN products p ON p.id = ci.product_id
		JOIN shops sh ON sh.id = p.shop_id
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
		var sellable bool

		err := rows.Scan(
			&item.ProductID,
			&item.ShopID,
			&item.Title,
			&item.Price,
			&item.Quantity,
			&available,
			&sellable,
		)

		if err != nil {
			rows.Close()
			return nil, err
		}

		itemCount++

		if itemCount > 100 ||
			!sellable ||
			item.Quantity < 1 ||
			item.Quantity > 99 ||
			available < item.Quantity ||
			item.Price < 1 ||
			item.Price > 100000000 {
			rows.Close()
			return nil, ErrStockUnavailable
		}

		lineTotal := item.Price * int64(item.Quantity)
		subtotal += lineTotal

		group := groups[item.ShopID]

		if group == nil {
			group = &checkoutGroup{
				ShopID: item.ShopID,
			}
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

	// Calculate seller shipping charges from database rules.
	shippingTotal, err := calculateCheckoutShipping(
		ctx,
		tx,
		groups,
		country,
	)

	if err != nil {
		return nil, err
	}

	grandTotal := subtotal + shippingTotal

	// Create parent order.
	var orderID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO orders (
			buyer_id,
			address_id,
			address_snapshot,
			idempotency_key,
			subtotal_cents,
			shipping_total_cents,
			grand_total_cents,
			currency,
			payment_status,
			status,
			expires_at
		)
		VALUES (
			$1, $2, $3::jsonb, $4,
			$5, $6, $7,
			'USD',
			'pending',
			'pending_payment',
			NOW() + INTERVAL '30 minutes'
		)
		ON CONFLICT (buyer_id, idempotency_key)
		DO NOTHING
		RETURNING id
	`,
		buyerID,
		addressID,
		addressSnapshot,
		key,
		subtotal,
		shippingTotal,
		grandTotal,
	).Scan(&orderID)

	if errors.Is(err, pgx.ErrNoRows) {
		// Another request completed checkout using the same key.
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return nil, rollbackErr
		}

		id, savedAddress, lookupErr := s.repo.FindByKey(
			ctx, buyerID, key,
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

	// Process sellers in a consistent order.
	shopIDs := make([]int64, 0, len(groups))

	for id := range groups {
		shopIDs = append(shopIDs, id)
	}

	sort.Slice(shopIDs, func(i, j int) bool {
		return shopIDs[i] < shopIDs[j]
	})

	// Create one suborder per seller.
	for _, shopID := range shopIDs {
		group := groups[shopID]
		var sellerOrderID int64

		err = tx.QueryRow(ctx, `
			INSERT INTO seller_orders (
				order_id,
				shop_id,
				subtotal_cents,
				shipping_cents,
				status
			)
			VALUES ($1, $2, $3, $4, 'pending_payment')
			RETURNING id
		`,
			orderID,
			shopID,
			group.Subtotal,
			group.Shipping,
		).Scan(&sellerOrderID)

		if err != nil {
			return nil, err
		}

		for _, item := range group.Items {
			var orderItemID int64

			err = tx.QueryRow(ctx, `
				INSERT INTO order_items (
					seller_order_id,
					product_id,
					product_title_snapshot,
					unit_price_cents,
					quantity,
					line_total_cents
				)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id
			`,
				sellerOrderID,
				item.ProductID,
				item.Title,
				item.Price,
				item.Quantity,
				item.Price*int64(item.Quantity),
			).Scan(&orderItemID)

			if err != nil {
				return nil, err
			}

			// Reserve product stock through Kanishka's module.
			if err := s.inventory.ReserveForCheckout(
				ctx,
				tx,
				item.ProductID,
				item.Quantity,
			); err != nil {
				return nil, fmt.Errorf(
					"%w: %v",
					ErrStockUnavailable,
					err,
				)
			}

			// Record each reservation separately.
			_, err = tx.Exec(ctx, `
				INSERT INTO stock_reservations (
					order_item_id,
					product_id,
					quantity,
					status
				)
				VALUES ($1, $2, $3, 'reserved')
			`,
				orderItemID,
				item.ProductID,
				item.Quantity,
			)

			if err != nil {
				return nil, err
			}
		}
	}

	// Record initial order status.
	_, err = tx.Exec(ctx, `
		INSERT INTO order_status_history (
			order_id,
			to_status,
			reason
		)
		VALUES (
			$1,
			'pending_payment',
			'checkout_created'
		)
	`, orderID)

	if err != nil {
		return nil, err
	}

	// Clear the buyer's cart after successful order creation.
	_, err = tx.Exec(ctx, `
		DELETE FROM cart_items
		WHERE cart_id = $1
	`, cartID)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.repo.GetBuyerOrder(ctx, buyerID, orderID)
}

// transitionReservations updates stock reservations.
// The caller must already lock the parent order.
func (s *Service) transitionReservations(
	ctx context.Context,
	tx pgx.Tx,
	orderID int64,
	action string,
) error {
	rows, err := tx.Query(ctx, `
		SELECT
			sr.id,
			sr.product_id,
			sr.quantity,
			sr.status
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

	reservations := make([]reservation, 0)

	for rows.Next() {
		var r reservation

		if err := rows.Scan(
			&r.ID,
			&r.ProductID,
			&r.Quantity,
			&r.Status,
		); err != nil {
			rows.Close()
			return err
		}

		reservations = append(reservations, r)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(reservations) == 0 {
		return ErrInvalidTransition
	}

	for _, r := range reservations {
		if r.Status != "reserved" {
			return ErrInvalidTransition
		}

		var next string

		switch action {
		case "commit":
			err = s.inventory.ConfirmCheckoutReservation(
				ctx,
				tx,
				r.ProductID,
				r.Quantity,
			)
			next = "committed"

		case "release":
			err = s.inventory.ReleaseCheckoutReservation(
				ctx,
				tx,
				r.ProductID,
				r.Quantity,
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
			SET
				status = $2,
				updated_at = NOW()
			WHERE id = $1
			  AND status = 'reserved'
		`, r.ID, next)

		if err != nil {
			return err
		}
	}

	return nil
}

// MarkPaidTx is an internal method, not a public API.
// Only the verified payment service should call it.
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
		SELECT
			status,
			payment_status,
			expires_at
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, orderID).Scan(
		&status,
		&paymentStatus,
		&expiresAt,
	)

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
		ctx,
		tx,
		orderID,
		"commit",
	); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET
			payment_status = 'paid',
			status = 'processing',
			updated_at = NOW()
		WHERE id = $1
	`, orderID)

	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE seller_orders
		SET
			status = 'processing',
			updated_at = NOW()
		WHERE order_id = $1
	`, orderID)

	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO order_status_history (
			order_id,
			from_status,
			to_status,
			reason
		)
		VALUES (
			$1,
			'pending_payment',
			'processing',
			'verified_payment'
		)
	`, orderID)

	return err
}

// CancelOrder allows the buyer to cancel an unpaid order.
func (s *Service) CancelOrder(
	ctx context.Context,
	buyerID, orderID int64,
) error {
	if buyerID <= 0 {
		return ErrInvalidOrder
	}

	return s.closePending(
		ctx,
		orderID,
		buyerID,
		"cancelled",
	)
}

// closePending handles buyer cancellation and automatic expiry.
// buyerID == 0 is reserved for the trusted expiry worker.
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
		SELECT
			buyer_id,
			status,
			payment_status,
			expires_at
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, orderID).Scan(
		&ownerID,
		&status,
		&paymentStatus,
		&expiresAt,
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

	if target == "expired" &&
		time.Now().Before(expiresAt) {
		return ErrInvalidTransition
	}

	if err := s.transitionReservations(
		ctx,
		tx,
		orderID,
		"release",
	); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE seller_orders
		SET
			status = 'cancelled',
			updated_at = NOW()
		WHERE order_id = $1
	`, orderID)

	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET
			status = $2,
			updated_at = NOW()
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
			order_id,
			from_status,
			to_status,
			actor_user_id,
			reason
		)
		VALUES (
			$1,
			'pending_payment',
			$2,
			$3,
			$4
		)
	`, orderID, target, actor, target)

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
