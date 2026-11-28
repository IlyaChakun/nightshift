package dispatch

import (
	"encoding/json"
	"testing"
)

// The check matches what only a check outside the browser settles, and leaves page words alone
// (review of the V4 fix: a "Pipeline" page or a "Commit" button is a browser scenario).
func TestNotInBrowser(t *testing.T) {
	for text, want := range map[string]bool{
		"the raw bytes of hello.txt start without a BOM":      true,
		"hello.txt compared against main differs in one line": true,
		"the CI passes on the branch":                         true,
		"git diff shows one file":                             true,
		"the visitor opens the Sales pipeline page":           false,
		"the owner presses Commit changes":                    false,
		"the Repository settings tab lists two members":       false,
		"the diff view shows the change in green":             false,
		"the page shows Hello, Ann":                           false,
		// V4 on the Mac, task #1015: acceptance opens only the preview.
		"on the production page after the rollback finished the list lacks broken-release": true,
		"once the release is done the banner is gone":                                      true,
		"the change was rolled back":                                                       true,
		"на странице продакшена список пуст":                                               true,
		"the visitor presses the Rollback button on the preview":                           false,
		"the Releases page lists v2":                                                       false,
		"the Production schedule page shows two rows":                                      false,
	} {
		if got := notInBrowser.MatchString(text); got != want {
			t.Errorf("%q: got %v, want %v", text, got, want)
		}
	}
}

func TestSplitObservable(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"hidden_scenarios": []scenario{
		{ID: "HS-1", Given: "the preview", When: "a visitor opens /", Then: "the page shows Hello"},
		{ID: "HS-2", Given: "the change", When: "the file is compared with main", Then: "one line differs"},
	}})
	keep, out := splitObservable(raw)
	if len(keep) != 1 || keep[0].ID != "HS-1" || len(out) != 1 || out[0] != "HS-2" {
		t.Fatalf("keep %+v, out %v", keep, out)
	}
}

func TestUniqueIDsSortByNumber(t *testing.T) {
	if got := uniqueIDs([]string{"HS-10", "HS-2", "HS-2"}); len(got) != 2 || got[0] != "HS-2" || got[1] != "HS-10" {
		t.Fatalf("got %v", got)
	}
}
