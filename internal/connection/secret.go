package connection

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/ktr0731/go-fuzzyfinder"
	"golang.org/x/term"
)

// rdsClusterTagKey is the tag key Amazon RDS automatically attaches to
// secrets it creates for "Manage master user password in AWS Secrets
// Manager"; the value is the owning DB cluster ARN.
const rdsClusterTagKey = "aws:rds:primaryDBClusterArn"

// SecretInfo is a flattened view of Secrets Manager ListSecrets entries.
type SecretInfo struct {
	Name        string
	ARN         string
	Description string
}

// ListSecrets returns all Secrets Manager secrets visible in the configured
// region. No filtering is applied because secret-to-cluster mapping is not
// expressed uniformly enough to filter safely.
func ListSecrets(ctx context.Context, cfg aws.Config) ([]SecretInfo, error) {
	client := secretsmanager.NewFromConfig(cfg)
	paginator := secretsmanager.NewListSecretsPaginator(client, &secretsmanager.ListSecretsInput{})

	var out []SecretInfo
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		for _, s := range page.SecretList {
			out = append(out, SecretInfo{
				Name:        aws.ToString(s.Name),
				ARN:         aws.ToString(s.ARN),
				Description: aws.ToString(s.Description),
			})
		}
	}
	return out, nil
}

// SuggestSecretsForCluster returns the Secrets Manager secrets that are
// most likely the right credentials for the given Aurora cluster. It looks
// in two places, in order:
//
//  1. The cluster's MasterUserSecret (RDS-managed password rotation). If
//     present this is the canonical answer.
//  2. Secrets tagged aws:rds:primaryDBClusterArn = <cluster ARN>, which is
//     how RDS marks the secret it created when "Manage master user password
//     in AWS Secrets Manager" was turned on.
//
// Errors from one path do not abort the other so the caller still gets
// whatever could be found. The result is deduped by ARN.
func SuggestSecretsForCluster(ctx context.Context, cfg aws.Config, cluster ClusterInfo) ([]SecretInfo, error) {
	smClient := secretsmanager.NewFromConfig(cfg)

	var (
		out      []SecretInfo
		seen     = map[string]struct{}{}
		firstErr error
	)

	add := func(info SecretInfo) {
		if info.ARN == "" {
			return
		}
		if _, dup := seen[info.ARN]; dup {
			return
		}
		seen[info.ARN] = struct{}{}
		out = append(out, info)
	}

	// 1. Cluster MasterUserSecret.
	if cluster.MasterUserSecretArn != "" {
		info := SecretInfo{
			ARN:         cluster.MasterUserSecretArn,
			Description: "MasterUserSecret (managed by RDS)",
			Name:        secretNameFromARN(cluster.MasterUserSecretArn),
		}
		// Best effort: pull the friendly name via DescribeSecret. If the
		// caller doesn't have secretsmanager:DescribeSecret we keep the
		// ARN-derived fallback name so the picker still has something
		// readable to display.
		desc, err := smClient.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
			SecretId: aws.String(cluster.MasterUserSecretArn),
		})
		if err == nil {
			if name := aws.ToString(desc.Name); name != "" {
				info.Name = name
			}
			if d := aws.ToString(desc.Description); d != "" {
				info.Description = d + " · MasterUserSecret"
			}
		} else {
			firstErr = fmt.Errorf("describe master user secret: %w", err)
		}
		add(info)
	}

	// 2. Secrets tagged with the cluster ARN.
	if cluster.ARN != "" {
		paginator := secretsmanager.NewListSecretsPaginator(smClient, &secretsmanager.ListSecretsInput{
			Filters: []smtypes.Filter{
				{
					Key:    smtypes.FilterNameStringTypeTagKey,
					Values: []string{rdsClusterTagKey},
				},
			},
		})
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("list secrets by tag: %w", err)
				}
				break
			}
			for _, s := range page.SecretList {
				for _, t := range s.Tags {
					if aws.ToString(t.Key) != rdsClusterTagKey {
						continue
					}
					if aws.ToString(t.Value) != cluster.ARN {
						continue
					}
					add(SecretInfo{
						Name:        aws.ToString(s.Name),
						ARN:         aws.ToString(s.ARN),
						Description: aws.ToString(s.Description),
					})
					break
				}
			}
		}
	}

	if len(out) > 0 {
		return out, nil
	}
	return nil, firstErr
}

// secretNameFromARN extracts a readable last segment from a secret ARN so
// the picker has something to show even when DescribeSecret is unavailable.
func secretNameFromARN(arn string) string {
	if idx := strings.LastIndex(arn, ":"); idx >= 0 && idx < len(arn)-1 {
		return arn[idx+1:]
	}
	return arn
}

// SelectSecret launches a fuzzy finder over the given secrets. The display
// string includes the description when present so that users can recognize
// the right secret even when names are cryptic.
func SelectSecret(secrets []SecretInfo) (SecretInfo, error) {
	if len(secrets) == 0 {
		return SecretInfo{}, errors.New("no Secrets Manager secrets found in this region")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return SecretInfo{}, errors.New("--secret requires a TTY for interactive selection; pass an ARN explicitly")
	}

	idx, err := fuzzyfinder.Find(secrets, func(i int) string {
		s := secrets[i]
		if s.Description != "" {
			return fmt.Sprintf("%s — %s", s.Name, s.Description)
		}
		return s.Name
	}, fuzzyfinder.WithPromptString("Secret> "))
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return SecretInfo{}, errors.New("secret selection aborted")
		}
		return SecretInfo{}, fmt.Errorf("fuzzy finder: %w", err)
	}
	return secrets[idx], nil
}
