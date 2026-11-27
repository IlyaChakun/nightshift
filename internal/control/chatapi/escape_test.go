package chatapi

import "testing"

func TestEscapeNeutralisesMarkup(t *testing.T) {
	if got := Escape("<!channel> a&b <u|x>"); got != "&lt;!channel&gt; a&amp;b &lt;u|x&gt;" {
		t.Fatalf("got %q", got)
	}
}
