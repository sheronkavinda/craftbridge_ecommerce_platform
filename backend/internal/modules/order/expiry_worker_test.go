package order

import (
	"context"
	"errors"
	"testing"
)

type fakeExpiryProcessor struct {
	counts []int
	calls  int
	failAt int
}

func (f *fakeExpiryProcessor) ExpirePendingOrders(
	ctx context.Context,
	limit int,
) (int, error) {
	f.calls++

	if f.failAt > 0 && f.calls == f.failAt {
		return 0, errors.New("simulated database error")
	}

	if f.calls > len(f.counts) {
		return 0, nil
	}

	return f.counts[f.calls-1], nil
}

func TestRunExpirySweep(t *testing.T) {
	fake := &fakeExpiryProcessor{
		counts: []int{50, 3},
	}

	total, err := RunExpirySweep(
		context.Background(),
		fake,
		50,
		10,
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if total != 53 {
		t.Fatalf(
			"Expected 53 expired orders, got %d",
			total,
		)
	}

	if fake.calls != 2 {
		t.Fatalf(
			"Expected 2 batches, got %d",
			fake.calls,
		)
	}
}

func TestRunExpirySweepStopsOnError(t *testing.T) {
	fake := &fakeExpiryProcessor{
		counts: []int{50},
		failAt: 2,
	}

	total, err := RunExpirySweep(
		context.Background(),
		fake,
		50,
		10,
	)

	if err == nil {
		t.Fatal("Expected an error")
	}

	if total != 50 {
		t.Fatalf(
			"Expected 50 processed orders, got %d",
			total,
		)
	}
}

func TestRunExpirySweepCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	fake := &fakeExpiryProcessor{}

	_, err := RunExpirySweep(ctx, fake, 50, 10)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"Expected context.Canceled, got %v",
			err,
		)
	}

	if fake.calls != 0 {
		t.Fatal("Cancelled worker should not run")
	}
}
