package server

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestVerifyFrontendEmbed_OK(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
	}
	if err := verifyFrontendEmbed(fs); err != nil {
		t.Fatalf("expected nil error when index.html is present, got %v", err)
	}
}

func TestVerifyFrontendEmbed_Missing(t *testing.T) {
	fs := fstest.MapFS{
		".gitkeep": {Data: []byte{}},
	}
	err := verifyFrontendEmbed(fs)
	if err == nil {
		t.Fatal("expected error when index.html is missing, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"embedded frontend is missing",
		"go install",
		"make build",
		"releases",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}
