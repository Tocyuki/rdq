package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setStatePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	t.Setenv("RDQ_STATE_FILE", path)
	return path
}

func TestLoadMissingFileReturnsEmptyState(t *testing.T) {
	setStatePath(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Profiles) != 0 {
		t.Errorf("expected empty profiles, got %v", s.Profiles)
	}
}

func TestLoadMalformedReturnsError(t *testing.T) {
	path := setStatePath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	setStatePath(t)

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Set("dev", ProfileState{
		Cluster:  "arn:aws:rds:ap-northeast-1:123:cluster:dev",
		Secret:   "arn:aws:secretsmanager:ap-northeast-1:123:secret:dev-abc",
		Database: "myapp",
	})
	s.Set("prod", ProfileState{
		Cluster:  "arn:aws:rds:us-east-1:456:cluster:prod",
		Secret:   "arn:aws:secretsmanager:us-east-1:456:secret:prod-xyz",
		Database: "core",
	})
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := loaded.Get("dev"); got.Cluster != s.Get("dev").Cluster || got.Database != "myapp" {
		t.Errorf("dev round-trip mismatch: %+v", got)
	}
	if got := loaded.Get("prod"); got.Cluster != s.Get("prod").Cluster || got.Database != "core" {
		t.Errorf("prod round-trip mismatch: %+v", got)
	}
}

func TestGetEmptyProfileMapsToDefault(t *testing.T) {
	setStatePath(t)
	s, _ := Load()
	s.Set("", ProfileState{Database: "fallback"})
	if got := s.Get(""); got.Database != "fallback" {
		t.Errorf("expected fallback, got %+v", got)
	}
	if got := s.Profiles["_default_"]; got.Database != "fallback" {
		t.Errorf("expected synthetic key _default_, got %+v", s.Profiles)
	}
}

func TestNormalizeHistoryDedupsAndPromotes(t *testing.T) {
	cases := []struct {
		name       string
		history    []string
		mostRecent string
		want       []string
	}{
		{
			name:       "promote mostRecent to front",
			history:    []string{"foo", "bar", "baz"},
			mostRecent: "bar",
			want:       []string{"bar", "foo", "baz"},
		},
		{
			name:       "new entry prepended",
			history:    []string{"foo", "bar"},
			mostRecent: "newdb",
			want:       []string{"newdb", "foo", "bar"},
		},
		{
			name:       "empty mostRecent leaves history",
			history:    []string{"foo", "foo", "bar"},
			mostRecent: "",
			want:       []string{"foo", "bar"},
		},
		{
			name:       "respects history limit",
			history:    []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"},
			mostRecent: "z",
			want:       []string{"z", "a", "b", "c", "d", "e", "f", "g", "h", "i"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeHistory(tc.history, tc.mostRecent)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetThroughSavePreservesHistory(t *testing.T) {
	setStatePath(t)
	s, _ := Load()
	s.Set("dev", ProfileState{Database: "first"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, _ := Load()
	ps := s2.Get("dev")
	ps.Database = "second"
	s2.Set("dev", ps)
	if err := s2.Save(); err != nil {
		t.Fatal(err)
	}

	s3, _ := Load()
	ps = s3.Get("dev")
	if ps.Database != "second" {
		t.Errorf("expected second, got %q", ps.Database)
	}
	want := []string{"second", "first"}
	if !reflect.DeepEqual(ps.DatabaseHistory, want) {
		t.Errorf("history mismatch: got %v, want %v", ps.DatabaseHistory, want)
	}
}
