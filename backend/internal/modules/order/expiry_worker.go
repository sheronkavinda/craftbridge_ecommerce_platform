package order

import (
	"context"
	"errors"
	"log"
	"time"
)

// ExpiryProcessor allows the worker to call
// the order service and also be unit-tested.
type ExpiryProcessor interface {
	ExpirePendingOrders(
		ctx context.Context,
		limit int,
	) (int, error)
}

// RunExpirySweep processes expired orders in batches.
func RunExpirySweep(
	ctx context.Context,
	processor ExpiryProcessor,
	batchSize int,
	maxBatches int,
) (int, error) {

	if processor == nil {
		return 0, errors.New("expiry processor is required")
	}

	if batchSize <= 0 || batchSize > 100 {
		batchSize = 50
	}

	if maxBatches <= 0 || maxBatches > 20 {
		maxBatches = 10
	}

	totalExpired := 0

	for batch := 0; batch < maxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return totalExpired, err
		}

		count, err := processor.ExpirePendingOrders(
			ctx,
			batchSize,
		)

		totalExpired += count

		if err != nil {
			return totalExpired, err
		}

		if count < batchSize {
			break
		}
	}

	return totalExpired, nil
}

// StartExpiryWorker runs the expiry check in the background.
// It is activated only when explicitly enabled by the server.
func StartExpiryWorker(
	ctx context.Context,
	processor ExpiryProcessor,
	interval time.Duration,
	batchSize int,
) {

	if interval < time.Second {
		interval = time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		runSweep := func() {
			if ctx.Err() != nil {
				return
			}

			sweepCtx, cancel := context.WithTimeout(
				ctx,
				30*time.Second,
			)
			defer cancel()

			expiredCount, err := RunExpirySweep(
				sweepCtx,
				processor,
				batchSize,
				10,
			)

			if err != nil {
				if ctx.Err() == nil {
					log.Printf(
						"Order expiry worker failed: %v",
						err,
					)
				}
				return
			}

			if expiredCount > 0 {
				log.Printf(
					"Expired %d pending orders",
					expiredCount,
				)
			}
		}

		// Check outstanding orders when the server starts.
		runSweep()

		for {
			select {
			case <-ctx.Done():
				log.Println("Order expiry worker stopped")
				return

			case <-ticker.C:
				runSweep()
			}
		}
	}()
}
