package awsauth

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListProfiles(t *testing.T) {
	cases := []struct {
		name        string
		config      string
		credentials string
		want        []string
	}{
		{
			name: "default only from config",
			config: `[default]
region = us-east-1
`,
			want: []string{"default"},
		},
		{
			name: "multiple profiles in config",
			config: `[default]
region = us-east-1

[profile dev]
region = us-west-2

[profile prod]
region = ap-northeast-1
`,
			want: []string{"default", "dev", "prod"},
		},
		{
			name: "merge config and credentials with dedup",
			config: `[default]
region = us-east-1

[profile dev]
region = us-west-2
`,
			credentials: `[default]
aws_access_key_id = AKIA...

[work]
aws_access_key_id = AKIA...
`,
			want: []string{"default", "dev", "work"},
		},
		{
			name: "credentials only (no default)",
			credentials: `[foo]
aws_access_key_id = AKIA...

[bar]
aws_access_key_id = AKIA...
`,
			want: []string{"bar", "foo"},
		},
		{
			name: "ignores malformed and comment lines",
			config: `# comment
[default]
region = us-east-1
not-a-section
[profile  ]
[profile good]
`,
			want: []string{"default", "good"},
		},
		{
			name: "both files missing",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config")
			credPath := filepath.Join(dir, "credentials")

			if tc.config != "" {
				if err := os.WriteFile(cfgPath, []byte(tc.config), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("AWS_CONFIG_FILE", cfgPath)
			} else {
				t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "no-config"))
			}

			if tc.credentials != "" {
				if err := os.WriteFile(credPath, []byte(tc.credentials), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credPath)
			} else {
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "no-credentials"))
			}

			got, err := ListProfiles()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ListProfiles() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListProfilesDefaultFirst(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	contents := `[profile zebra]
[profile alpha]
[default]
[profile mike]
`
	if err := os.WriteFile(cfgPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", cfgPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "no-credentials"))

	got, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"default", "alpha", "mike", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListProfiles() = %v, want %v", got, want)
	}
}
