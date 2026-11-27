package pgtest

import (
	"os"
	"strings"
	"testing"
)

// TestImageIsTheComposeImage: tests run on the Postgres that production runs, pgvector included.
func TestImageIsTheComposeImage(t *testing.T) {
	compose, err := os.ReadFile("../../../../deploy/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "image: "+Image+"\n") {
		t.Fatalf("deploy/docker-compose.yml does not run %s; keep pgtest.Image equal to its postgres image", Image)
	}
}
