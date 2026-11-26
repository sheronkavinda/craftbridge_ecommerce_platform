package payment

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// OrderTxBeginner starts a pgx transaction for order state transitions.
type OrderTxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// OrderPaidMarker marks an order paid inside an existing database transaction.
type OrderPaidMarker interface {
	MarkPaidTx(ctx context.Context, tx pgx.Tx, orderID int64) error
}

// OrderServiceNotifier implements OrderNotifier by calling Member 04's order.Service.MarkPaidTx
// inside a pgx transaction when a payment reaches SUCCESS.
type OrderServiceNotifier struct {
	txBeginner           OrderTxBeginner
	orderMarker          OrderPaidMarker
	errInvalidTransition error
}

// NewOrderServiceNotifier creates an OrderServiceNotifier backed by a pgx pool and order service.
func NewOrderServiceNotifier(
	txBeginner OrderTxBeginner,
	orderMarker OrderPaidMarker,
	errInvalidTransition error,
) *OrderServiceNotifier {
	return &OrderServiceNotifier{
		txBeginner:           txBeginner,
		orderMarker:          orderMarker,
		errInvalidTransition: errInvalidTransition,
	}
}

// NotifyPaymentResult transitions the order to paid/processing when Outcome is SUCCESS.
// Non-success outcomes (FAILED/CANCELLED/EXPIRED) leave the order unchanged so payment retries remain possible.
// Late/cancelled/expired transitions (order.ErrInvalidTransition) log a WARNING and return nil.
func (n *OrderServiceNotifier) NotifyPaymentResult(ctx context.Context, result PaymentResult) error {
	if result.Outcome != PaymentStatusSuccess {
		return nil
	}
	if n == nil || n.txBeginner == nil || n.orderMarker == nil {
		return errors.New("order service notifier is not configured")
	}

	orderID, err := strconv.ParseInt(strings.TrimSpace(result.OrderID), 10, 64)
	if err != nil || orderID <= 0 {
		log.Printf(
			"[req_id=%s] WARNING payment.OrderServiceNotifier: non-numeric order_id=%q attempt_id=%s; skipping MarkPaidTx",
			reqIDFromContext(ctx),
			result.OrderID,
			result.AttemptID,
		)
		return nil
	}

	tx, err := n.txBeginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin order mark-paid tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := n.orderMarker.MarkPaidTx(ctx, tx, orderID); err != nil {
		if n.errInvalidTransition != nil && errors.Is(err, n.errInvalidTransition) {
			log.Printf(
				"[req_id=%s] WARNING late payment for non-payable order: order_id=%d attempt_id=%s (%v)",
				reqIDFromContext(ctx),
				orderID,
				result.AttemptID,
				err,
			)
			return nil
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit order mark-paid tx: %w", err)
	}
	return nil
}
