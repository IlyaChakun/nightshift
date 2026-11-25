package main

import (
	"bytes"
	"testing"
)

func TestCheckRubricFlagsGoTogether(t *testing.T) {
	for _, args := range [][]string{
		{"check", "--out", "x", "--standards", "standards"},
		{"check", "--out", "x", "--judge-auth", "api_key"},
		{"check", "--out", "x", "--standards", "standards", "--judge-auth", "bogus"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("%v: code %d, %s", args, code, stderr.String())
		}
	}
}
