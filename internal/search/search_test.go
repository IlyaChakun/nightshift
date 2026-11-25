package search

import (
	"reflect"
	"testing"
)

func ids(hs []Hit) []int64 {
	var out []int64
	for _, h := range hs {
		out = append(out, h.ID)
	}
	return out
}

func TestFuse(t *testing.T) {
	text := []Hit{{ID: 1}, {ID: 2}, {ID: 3}}
	near := []Hit{{ID: 3}, {ID: 4}, {ID: 1}}
	// 1: 1/61 + 1/63; 3: 1/63 + 1/61 (a tie, broken by id); 2: 1/62; 4: 1/62.
	if got, want := ids(Fuse(text, near)), []int64{1, 3, 2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fused %v, want %v", got, want)
	}
	if got := ids(Fuse(text)); !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("one list keeps its order: %v", got)
	}
	if got := Fuse(); len(got) != 0 {
		t.Fatalf("no lists: %v", got)
	}
}

func TestVectorText(t *testing.T) {
	if got := vectorText([]float32{1, 0.5, -2}); got != "[1,0.5,-2]" {
		t.Fatalf("got %q", got)
	}
}
