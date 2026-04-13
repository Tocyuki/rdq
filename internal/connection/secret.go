package connection

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/ktr0731/go-fuzzyfinder"
	"golang.org/x/term"
)

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
