package learn

import (
	"reflect"
	"testing"
)

func TestRecurring(t *testing.T) {
	fs := []Finding{
		{TaskID: 1, RuleID: "go.errcheck", Category: "code", LayerCaught: "L1", Message: "unchecked"},
		{TaskID: 2, RuleID: "go.errcheck", Category: "code", LayerCaught: "L3", LayerExpected: "L1", Message: "unchecked"},
		{TaskID: 1, RuleID: "go.nilerr", Category: "code", LayerCaught: "L1", Message: "a"},
		{TaskID: 1, RuleID: "go.nilerr", Category: "code", LayerCaught: "L1", Message: "b"}, // same task twice: a loop
		{TaskID: 3, RuleID: "new", Category: "docs", LayerCaught: "L5", Message: "The ADR lacks alternatives."},
		{TaskID: 4, RuleID: "new", Category: "docs", LayerCaught: "L5", Message: "the adr lacks alternatives, again"},
		{TaskID: 5, RuleID: "old.rule", Category: "code", LayerCaught: "L1", Message: "x"},
		{TaskID: 6, RuleID: "old.rule", Category: "code", LayerCaught: "L1", Message: "x"},
	}
	got := Recurring(fs, 2, 3, map[string]bool{"old.rule": true})
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	want := []string{"go.errcheck", "new:docs:the-adr-lacks-alternatives"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("clusters %v, want %v", keys, want)
	}
	if !got[0].Late || got[0].Tasks != 2 || !reflect.DeepEqual(got[0].Layers, []string{"L1", "L3"}) {
		t.Errorf("first cluster: %+v", got[0])
	}
	if got[1].Count != 2 || len(got[1].Samples) != 2 {
		t.Errorf("second cluster: %+v", got[1])
	}
}

func TestRecurringLimit(t *testing.T) {
	var fs []Finding
	for _, r := range []string{"a", "b", "c", "d"} {
		fs = append(fs, Finding{TaskID: 1, RuleID: r, LayerCaught: "L1"}, Finding{TaskID: 2, RuleID: r, LayerCaught: "L1"})
	}
	if n := len(Recurring(fs, 2, 3, nil)); n != 3 {
		t.Fatalf("got %d clusters, want the limit of 3", n)
	}
}
