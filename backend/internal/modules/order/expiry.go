
package order

import (
	"context"
	"errors"
)

// ExpirePendingOrders releases reservations for pending
// orders whose payment window has ended.
//
// Call this from a scheduled job during integration.
// It is not exposed directly to buyers.
func (s *Service) ExpirePendingOrders(
	ctx context.Context,
	limit int,
) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.Query(ctx, `
		SELECT id
		FROM orders
		WHERE status = 'pending_payment'
		  AND payment_status = 'pending'
		  AND expires_at <= NOW()
		ORDER BY expires_at, id
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, err
	}

	ids := make([]int64, 0)

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	expired := 0

	for _, id := range ids {
		err := s.closePending(ctx, id, 0, "expired")

		// Another transaction may have completed/cancelled it.
		if errors.Is(err, ErrInvalidTransition) {
			continue
		}
		if err != nil {
			return expired, err
		}

		expired++
	}

	return expired, nil
}
