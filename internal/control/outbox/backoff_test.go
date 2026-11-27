package outbox

import "testing"

func TestBackoff(t *testing.T) {
	t.Parallel()
	for attempts, want := range map[int32]string{1: "5s", 2: "10s", 3: "20s", 9: "21m20s", 20: "1h0m0s"} {
		if got := Backoff(attempts).String(); got != want {
			t.Errorf("Backoff(%d) = %s, want %s", attempts, got, want)
		}
	}
}
