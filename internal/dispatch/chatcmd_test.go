package dispatch

import "testing"

func TestSlackEscapeNeutralisesMarkup(t *testing.T) {
	if got := slackEscape("<!channel> a&b <u|x>"); got != "&lt;!channel&gt; a&amp;b &lt;u|x&gt;" {
		t.Fatalf("got %q", got)
	}
}
