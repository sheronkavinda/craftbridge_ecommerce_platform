
package shipping

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidShipment = errors.New("invalid shipment update")
	ErrShipmentNotFound = errors.New("shipment or order not found")
	ErrFulfillmentState = errors.New("invalid fulfillment state")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

type SellerOrderView struct {
	ID              int64           `json:"id"`
	OrderID         int64           `json:"order_id"`
	ShopID          int64           `json:"shop_id"`
	SubtotalCents   int64           `json:"subtotal_cents"`
	ShippingCents   int64           `json:"shipping_cents"`
	Status          string          `json:"status"`
	PaymentStatus   string          `json:"payment_status"`
	AddressSnapshot json.RawMessage `json:"address_snapshot"`
}

type FulfillmentRequest struct {
	Status         string `json:"status" binding:"required"`
	Carrier        string `json:"carrier"`
	TrackingNumber string `json:"tracking_number"`
	Note           string `json:"note"`
}

type ShipmentEvent struct {
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type TrackingInfo struct {
	SellerOrderID int64           `json:"seller_order_id"`
	ShopName      string          `json:"shop_name"`
	Status        string          `json:"fulfillment_status"`
	Carrier       *string         `json:"carrier"`
	TrackingNo    *string         `json:"tracking_number"`
	Events        []ShipmentEvent `json:"events"`
}

// List only the seller's own orders.
func (s *Service) ListSellerOrders(
	ctx context.Context,
	sellerUserID int64,
) ([]SellerOrderView, error) {
	if sellerUserID <= 0 {
		return nil, ErrInvalidShipment
	}

	rows, err := s.db.Query(ctx, `
		SELECT so.id, so.order_id, so.shop_id,
		       so.subtotal_cents, so.shipping_cents,
		       so.status, o.payment_status,
		       o.address_snapshot::text
		FROM seller_orders so
		JOIN shops shop ON shop.id = so.shop_id
		JOIN orders o ON o.id = so.order_id
		WHERE shop.owner_user_id = $1
		  AND o.payment_status = 'paid'
		ORDER BY so.created_at DESC, so.id DESC
		LIMIT 100
	`, sellerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]SellerOrderView, 0)

	for rows.Next() {
		var item SellerOrderView
		var address string

		err := rows.Scan(
			&item.ID, &item.OrderID, &item.ShopID,
			&item.SubtotalCents, &item.ShippingCents,
			&item.Status, &item.PaymentStatus,
			&address,
		)
		if err != nil {
			return nil, err
		}

		item.AddressSnapshot = json.RawMessage(address)
		result = append(result, item)
	}

	return result, rows.Err()
}

// Update seller fulfilment after verified payment.
func (s *Service) UpdateFulfillment(
	ctx context.Context,
	sellerUserID, sellerOrderID int64,
	req FulfillmentRequest,
) error {
	if sellerUserID <= 0 || sellerOrderID <= 0 {
		return ErrInvalidShipment
	}

	req.Status = strings.TrimSpace(req.Status)
	req.Carrier = strings.TrimSpace(req.Carrier)
	req.TrackingNumber = strings.TrimSpace(req.TrackingNumber)
	req.Note = strings.TrimSpace(req.Note)

	if req.Status != "shipped" && req.Status != "delivered" {
		return ErrInvalidShipment
	}
	if len(req.Carrier) > 100 ||
		len(req.TrackingNumber) > 150 ||
		len(req.Note) > 255 {
		return ErrInvalidShipment
	}
	if req.Status == "shipped" &&
		(req.Carrier == "" || req.TrackingNumber == "") {
		return ErrInvalidShipment
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Verify the authenticated seller owns this suborder.
	var orderID int64
	err = tx.QueryRow(ctx, `
		SELECT so.order_id
		FROM seller_orders so
		JOIN shops shop ON shop.id = so.shop_id
		WHERE so.id = $1 AND shop.owner_user_id = $2
	`, sellerOrderID, sellerUserID).Scan(&orderID)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrShipmentNotFound
	}
	if err != nil {
		return err
	}

	// Lock parent order first to serialize payment,
	// cancellation, and parallel seller delivery updates.
	var parentStatus, paymentStatus string
	err = tx.QueryRow(ctx, `
		SELECT status, payment_status
		FROM orders WHERE id = $1 FOR UPDATE
	`, orderID).Scan(&parentStatus, &paymentStatus)
	if err != nil {
		return err
	}

	if paymentStatus != "paid" ||
		(parentStatus != "processing" &&
			parentStatus != "completed") {
		return ErrFulfillmentState
	}

	var currentStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM seller_orders
		WHERE id = $1 FOR UPDATE
	`, sellerOrderID).Scan(&currentStatus)
	if err != nil {
		return err
	}

	// Duplicate status requests have no second effect.
	if currentStatus == req.Status {
		return nil
	}

	if req.Status == "shipped" && currentStatus != "processing" {
		return ErrFulfillmentState
	}

	if req.Status == "delivered" && currentStatus != "shipped" {
		return ErrFulfillmentState
	}

	_, err = tx.Exec(ctx, `
		UPDATE seller_orders
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, sellerOrderID, req.Status)
	if err != nil {
		return err
	}

	var shipmentID int64

	if req.Status == "shipped" {
		err = tx.QueryRow(ctx, `
			INSERT INTO shipments (
				seller_order_id, carrier, tracking_number,
				status, shipped_at
			)
			VALUES ($1, $2, $3, 'shipped', NOW())
			ON CONFLICT (seller_order_id)
			DO UPDATE SET
				carrier = EXCLUDED.carrier,
				tracking_number = EXCLUDED.tracking_number,
				status = 'shipped',
				shipped_at = NOW(),
				updated_at = NOW()
			RETURNING id
		`, sellerOrderID, req.Carrier,
			req.TrackingNumber).Scan(&shipmentID)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE shipments
			SET status = 'delivered',
			    delivered_at = NOW(),
			    updated_at = NOW()
			WHERE seller_order_id = $1
			  AND status = 'shipped'
			RETURNING id
		`, sellerOrderID).Scan(&shipmentID)
	}

	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO shipment_events (
			shipment_id, status, note
		)
		VALUES ($1, $2, $3)
	`, shipmentID, req.Status, req.Note)
	if err != nil {
		return err
	}

	if req.Status == "delivered" {
		var outstanding int64

		err = tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM seller_orders
			WHERE order_id = $1 AND status <> 'delivered'
		`, orderID).Scan(&outstanding)
		if err != nil {
			return err
		}

		if outstanding == 0 && parentStatus != "completed" {
			_, err = tx.Exec(ctx, `
				UPDATE orders
				SET status = 'completed', updated_at = NOW()
				WHERE id = $1
			`, orderID)
			if err != nil {
				return err
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO order_status_history (
					order_id, from_status, to_status,
					actor_user_id, reason
				)
				VALUES (
					$1, 'processing', 'completed',
					$2, 'all_sellers_delivered'
				)
			`, orderID, sellerUserID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

// GetBuyerTracking ensures only the buyer can view
// shipment information for the parent order.
func (s *Service) GetBuyerTracking(
	ctx context.Context,
	buyerID, orderID int64,
) ([]TrackingInfo, error) {
	if buyerID <= 0 || orderID <= 0 {
		return nil, ErrInvalidShipment
	}

	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM orders
			WHERE id = $1 AND buyer_id = $2
		)
	`, orderID, buyerID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrShipmentNotFound
	}

	rows, err := s.db.Query(ctx, `
		SELECT so.id, shop.name, so.status,
		       sh.carrier, sh.tracking_number,
		       sh.id
		FROM seller_orders so
		JOIN shops shop ON shop.id = so.shop_id
		LEFT JOIN shipments sh ON sh.seller_order_id = so.id
		WHERE so.order_id = $1
		ORDER BY so.id
	`, orderID)
	if err != nil {
		return nil, err
	}

	type trackingRow struct {
		Info       TrackingInfo
		ShipmentID *int64
	}

	records := make([]trackingRow, 0)

	for rows.Next() {
		var record trackingRow

		err := rows.Scan(
			&record.Info.SellerOrderID,
			&record.Info.ShopName,
			&record.Info.Status,
			&record.Info.Carrier,
			&record.Info.TrackingNo,
			&record.ShipmentID,
		)
		if err != nil {
			rows.Close()
			return nil, err
		}

		record.Info.Events = make([]ShipmentEvent, 0)
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	result := make([]TrackingInfo, 0, len(records))

	for _, record := range records {
		if record.ShipmentID != nil {
			eventRows, err := s.db.Query(ctx, `
				SELECT status, COALESCE(note, ''),
				       created_at
				FROM shipment_events
				WHERE shipment_id = $1
				ORDER BY created_at, id
			`, *record.ShipmentID)
			if err != nil {
				return nil, err
			}

			for eventRows.Next() {
				var event ShipmentEvent
				err := eventRows.Scan(
					&event.Status, &event.Note,
					&event.CreatedAt,
				)
				if err != nil {
					eventRows.Close()
					return nil, err
				}
				record.Info.Events = append(
					record.Info.Events, event,
				)
			}

			if err := eventRows.Err(); err != nil {
				eventRows.Close()
				return nil, err
			}
			eventRows.Close()
		}

		result = append(result, record.Info)
	}

	return result, nil
}
