package order

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/jackc/pgx/v5"
)

var ErrShippingUnavailable = errors.New(
	"shipping not available for this seller and destination",
)

// Calculate a seller's shipping fee using integer cents.
// One base fee is charged per seller suborder.
func calculateShippingFee(
	baseCents, extraCents, quantity int64,
) (int64, error) {
	if baseCents < 0 ||
		extraCents < 0 ||
		quantity < 1 {
		return 0, ErrInvalidOrder
	}

	extraUnits := quantity - 1

	if extraCents > 0 &&
		extraUnits > (math.MaxInt64-baseCents)/extraCents {
		return 0, ErrInvalidOrder
	}

	return baseCents + extraUnits*extraCents, nil
}

// Read shipping rules inside the ongoing checkout transaction.
// All fees come from PostgreSQL, never from the buyer's JSON.
func calculateCheckoutShipping(
	ctx context.Context,
	tx pgx.Tx,
	groups map[int64]*checkoutGroup,
	country string,
) (int64, error) {
	if tx == nil || len(groups) == 0 {
		return 0, ErrInvalidOrder
	}

	shopIDs := make([]int64, 0, len(groups))

	for shopID := range groups {
		shopIDs = append(shopIDs, shopID)
	}

	sort.Slice(shopIDs, func(i, j int) bool {
		return shopIDs[i] < shopIDs[j]
	})

	var total int64

	for _, shopID := range shopIDs {
		group := groups[shopID]

		var base, extra int64

		err := tx.QueryRow(ctx, `
			SELECT base_fee_cents,
			       additional_item_fee_cents
			FROM seller_shipping_rates
			WHERE shop_id = $1
			  AND destination_country = $2
			  AND is_active = TRUE
		`, shopID, country).Scan(&base, &extra)

		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf(
				"shop %d: %w",
				shopID, ErrShippingUnavailable,
			)
		}

		if err != nil {
			return 0, err
		}

		var quantity int64

		for _, item := range group.Items {
			quantity += int64(item.Quantity)
		}

		fee, err := calculateShippingFee(
			base, extra, quantity,
		)
		if err != nil {
			return 0, err
		}

		if fee > math.MaxInt64-total {
			return 0, ErrInvalidOrder
		}

		group.Shipping = fee
		total += fee
	}

	return total, nil
}
