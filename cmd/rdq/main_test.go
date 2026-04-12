package main

import (
	"reflect"
	"testing"
)

func TestPreScanProfileArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "no args",
			in:   []string{"rdq"},
			want: []string{"rdq"},
		},
		{
			name: "unrelated args untouched",
			in:   []string{"rdq", "exec", "SELECT 1"},
			want: []string{"rdq", "exec", "SELECT 1"},
		},
		{
			name: "bare --profile at end",
			in:   []string{"rdq", "--profile"},
			want: []string{"rdq", "--profile=__rdq_select__"},
		},
		{
			name: "bare -p at end",
			in:   []string{"rdq", "-p"},
			want: []string{"rdq", "--profile=__rdq_select__"},
		},
		{
			name: "--profile with value",
			in:   []string{"rdq", "--profile", "foo"},
			want: []string{"rdq", "--profile", "foo"},
		},
		{
			name: "-p with value",
			in:   []string{"rdq", "-p", "foo"},
			want: []string{"rdq", "-p", "foo"},
		},
		{
			name: "--profile=value form untouched",
			in:   []string{"rdq", "--profile=foo"},
			want: []string{"rdq", "--profile=foo"},
		},
		{
			name: "bare --profile followed by subcommand",
			in:   []string{"rdq", "--profile", "exec", "SELECT 1"},
			want: []string{"rdq", "--profile=__rdq_select__", "exec", "SELECT 1"},
		},
		{
			name: "bare --profile followed by another flag",
			in:   []string{"rdq", "--profile", "-d"},
			want: []string{"rdq", "--profile=__rdq_select__", "-d"},
		},
		{
			name: "subcommand before --profile",
			in:   []string{"rdq", "exec", "--profile", "foo", "SELECT 1"},
			want: []string{"rdq", "exec", "--profile", "foo", "SELECT 1"},
		},
		{
			name: "bare -p followed by subcommand tui",
			in:   []string{"rdq", "-p", "tui"},
			want: []string{"rdq", "--profile=__rdq_select__", "tui"},
		},
		{
			name: "debug flag before bare --profile",
			in:   []string{"rdq", "-d", "--profile"},
			want: []string{"rdq", "-d", "--profile=__rdq_select__"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := preScanProfileArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("preScanProfileArgs(%v)\n  got:  %v\n  want: %v", tc.in, got, tc.want)
			}
		})
	}
}
