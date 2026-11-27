package jobsapi_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

func TestPriceCost(t *testing.T) {
	t.Parallel()
	var none *jobsapi.Price
	tests := []struct {
		name string
		p    *jobsapi.Price
		in   int
		out  int
		want float64
	}{
		{"no price is unknown", none, 1000, 1000, 0},
		{"input and output", &jobsapi.Price{InputPerMTok: 3, OutputPerMTok: 15}, 2_000_000, 1_000_000, 21},
		{"no tokens", &jobsapi.Price{InputPerMTok: 3, OutputPerMTok: 15}, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.p.Cost(tt.in, tt.out); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
