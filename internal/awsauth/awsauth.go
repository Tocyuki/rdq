// Package awsauth resolves AWS credentials for the rdq CLI.
//
// It wraps the SDK's default credential chain and adds an interactive
// profile picker for the "rdq --profile" (bare flag) UX.
package awsauth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ktr0731/go-fuzzyfinder"
	"golang.org/x/term"
)

// ListProfiles returns the profile names found in the shared AWS config and
// credentials files. "default" is always included as the first entry when it
// exists in either file. A missing file is not an error — it contributes no
// profiles.
func ListProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	set := make(map[string]struct{})

	if path := os.Getenv("AWS_CONFIG_FILE"); path != "" {
		if err := readConfigProfiles(path, set); err != nil {
			return nil, err
		}
	} else if err := readConfigProfiles(filepath.Join(home, ".aws", "config"), set); err != nil {
		return nil, err
	}

	if path := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); path != "" {
		if err := readCredentialsProfiles(path, set); err != nil {
			return nil, err
		}
	} else if err := readCredentialsProfiles(filepath.Join(home, ".aws", "credentials"), set); err != nil {
		return nil, err
	}

	if len(set) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(set))
	for name := range set {
		if name != "default" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	if _, ok := set["default"]; ok {
		names = append([]string{"default"}, names...)
	}
	return names, nil
}

// readConfigProfiles parses ~/.aws/config. Sections look like
// "[default]" or "[profile foo]"; the "profile " prefix is stripped.
func readConfigProfiles(path string, set map[string]struct{}) error {
	return scanIniSections(path, set, func(section string) (string, bool) {
		if section == "default" {
			return "default", true
		}
		if strings.HasPrefix(section, "profile ") {
			name := strings.TrimSpace(strings.TrimPrefix(section, "profile "))
			if name != "" {
				return name, true
			}
		}
		return "", false
	})
}

// readCredentialsProfiles parses ~/.aws/credentials. Sections are the raw
// profile name, e.g. "[default]" or "[foo]".
func readCredentialsProfiles(path string, set map[string]struct{}) error {
	return scanIniSections(path, set, func(section string) (string, bool) {
		if section == "" {
			return "", false
		}
		return section, true
	})
}

func scanIniSections(path string, set map[string]struct{}, extract func(string) (string, bool)) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		section := strings.TrimSpace(line[1 : len(line)-1])
		if name, ok := extract(section); ok {
			set[name] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

// SelectProfile launches an interactive fuzzy finder over the given profile
// names. It fails early when stdin is not a terminal, since the finder would
// otherwise corrupt piped input.
func SelectProfile(profiles []string) (string, error) {
	if len(profiles) == 0 {
		return "", errors.New("no AWS profiles found in ~/.aws/config or ~/.aws/credentials")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("--profile requires a TTY for interactive selection; pass a value explicitly or set AWS_PROFILE")
	}

	idx, err := fuzzyfinder.Find(profiles, func(i int) string {
		return profiles[i]
	}, fuzzyfinder.WithPromptString("AWS profile> "))
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return "", errors.New("profile selection aborted")
		}
		return "", fmt.Errorf("fuzzy finder: %w", err)
	}
	return profiles[idx], nil
}

// LoadConfig resolves AWS credentials. When profile is empty the SDK's default
// chain applies (environment variables → shared config default profile →
// EC2/ECS metadata). When profile is non-empty it is used as the shared config
// profile override.
func LoadConfig(ctx context.Context, profile string) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load aws config: %w", err)
	}
	return cfg, nil
}

// VerifyIdentity calls sts:GetCallerIdentity and returns the account and ARN
// for debug logging.
func VerifyIdentity(ctx context.Context, cfg aws.Config) (account, arn string, err error) {
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", fmt.Errorf("sts GetCallerIdentity: %w", err)
	}
	return aws.ToString(out.Account), aws.ToString(out.Arn), nil
}
