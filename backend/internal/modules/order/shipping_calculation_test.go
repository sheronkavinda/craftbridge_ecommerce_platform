package order

import "testing"

func TestCalculateShippingFee(t *testing.T) {
	tests := []struct {
		name     string
		base     int64
		extra    int64
		quantity int64
		expected int64
		wantErr  bool
	}{
		{
			name:     "One item",
			base:     700,
			extra:    100,
			quantity: 1,
			expected: 700,
		},
		{
			name:     "Two items",
			base:     700,
			extra:    100,
			quantity: 2,
			expected: 800,
		},
		{
			name:     "Five items",
			base:     700,
			extra:    100,
			quantity: 5,
			expected: 1100,
		},
		{
			name:     "Free shipping",
			base:     0,
			extra:    0,
			quantity: 3,
			expected: 0,
		},
		{
			name:     "Invalid zero quantity",
			base:     700,
			extra:    100,
			quantity: 0,
			wantErr:  true,
		},
		{
			name:     "Invalid negative fee",
			base:     -100,
			extra:    100,
			quantity: 2,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateShippingFee(
				tt.base,
				tt.extra,
				tt.quantity,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected validation error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if got != tt.expected {
				t.Fatalf(
					"Expected %d cents, received %d",
					tt.expected,
					got,
				)
			}
		})
	}
}
