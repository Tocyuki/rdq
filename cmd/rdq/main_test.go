package main

import (
	"reflect"
	"testing"
)

func TestPreScanBareFlags(t *testing.T) {
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

		// --profile / -p
		{
			name: "bare --profile at end",
			in:   []string{"rdq", "--profile"},
			want: []string{"rdq", "--profile=__rdq_profile_select__"},
		},
		{
			name: "bare -p at end",
			in:   []string{"rdq", "-p"},
			want: []string{"rdq", "--profile=__rdq_profile_select__"},
		},
		{
			name: "--profile with value",
			in:   []string{"rdq", "--profile", "foo"},
			want: []string{"rdq", "--profile", "foo"},
		},
		{
			name: "--profile=value form untouched",
			in:   []string{"rdq", "--profile=foo"},
			want: []string{"rdq", "--profile=foo"},
		},
		{
			name: "bare --profile followed by subcommand",
			in:   []string{"rdq", "--profile", "exec", "SELECT 1"},
			want: []string{"rdq", "--profile=__rdq_profile_select__", "exec", "SELECT 1"},
		},
		{
			name: "bare --profile followed by another flag",
			in:   []string{"rdq", "--profile", "-d"},
			want: []string{"rdq", "--profile=__rdq_profile_select__", "-d"},
		},

		// --cluster
		{
			name: "bare --cluster at end",
			in:   []string{"rdq", "--cluster"},
			want: []string{"rdq", "--cluster=__rdq_cluster_select__"},
		},
		{
			name: "--cluster with arn value",
			in:   []string{"rdq", "--cluster", "arn:aws:rds:::cluster:foo"},
			want: []string{"rdq", "--cluster", "arn:aws:rds:::cluster:foo"},
		},
		{
			name: "--cluster=arn untouched",
			in:   []string{"rdq", "--cluster=arn:aws:rds:::cluster:foo"},
			want: []string{"rdq", "--cluster=arn:aws:rds:::cluster:foo"},
		},
		{
			name: "bare --cluster followed by subcommand",
			in:   []string{"rdq", "--cluster", "exec"},
			want: []string{"rdq", "--cluster=__rdq_cluster_select__", "exec"},
		},

		// --secret
		{
			name: "bare --secret at end",
			in:   []string{"rdq", "--secret"},
			want: []string{"rdq", "--secret=__rdq_secret_select__"},
		},
		{
			name: "--secret with value",
			in:   []string{"rdq", "--secret", "arn:aws:secretsmanager:::secret:foo"},
			want: []string{"rdq", "--secret", "arn:aws:secretsmanager:::secret:foo"},
		},

		// --database
		{
			name: "bare --database at end",
			in:   []string{"rdq", "--database"},
			want: []string{"rdq", "--database=__rdq_database_select__"},
		},
		{
			name: "--database with value",
			in:   []string{"rdq", "--database", "myapp"},
			want: []string{"rdq", "--database", "myapp"},
		},

		// Mixed bare flags
		{
			name: "all four bare flags before subcommand",
			in:   []string{"rdq", "--profile", "--cluster", "--secret", "--database", "exec", "SELECT 1"},
			want: []string{"rdq", "--profile=__rdq_profile_select__", "--cluster=__rdq_cluster_select__", "--secret=__rdq_secret_select__", "--database=__rdq_database_select__", "exec", "SELECT 1"},
		},
		{
			name: "bare --profile and explicit --cluster",
			in:   []string{"rdq", "--profile", "--cluster", "arn:foo", "exec", "SELECT 1"},
			want: []string{"rdq", "--profile=__rdq_profile_select__", "--cluster", "arn:foo", "exec", "SELECT 1"},
		},
		{
			name: "subcommand before flag is preserved",
			in:   []string{"rdq", "exec", "--profile", "foo", "SELECT 1"},
			want: []string{"rdq", "exec", "--profile", "foo", "SELECT 1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := preScanBareFlags(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("preScanBareFlags(%v)\n  got:  %v\n  want: %v", tc.in, got, tc.want)
			}
		})
	}
}
