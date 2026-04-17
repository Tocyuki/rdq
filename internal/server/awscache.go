package server

import (
	"context"
	"sync"

	"github.com/Tocyuki/rdq/internal/awsauth"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// awsCache is a per-profile aws.Config cache. LoadConfig can take tens of
// seconds for SSO profiles on first use because it triggers a browser-based
// device-code flow, so we memoize the result for the remainder of the server
// process. A per-profile mutex prevents concurrent in-flight loads for the
// same profile from each opening their own SSO window.
//
// Errors are deliberately not cached — an SSO login failure should be
// retryable on the next request once the user finishes in the browser.
type awsCache struct {
	mu      sync.Mutex
	configs map[string]aws.Config
	locks   map[string]*sync.Mutex

	// loader is swappable for tests. Defaults to awsauth.LoadConfig.
	loader func(ctx context.Context, profile string) (aws.Config, error)
}

func newAWSCache() *awsCache {
	return &awsCache{
		configs: map[string]aws.Config{},
		locks:   map[string]*sync.Mutex{},
		loader:  awsauth.LoadConfig,
	}
}

// Get returns the cached aws.Config for profile, loading it the first time
// (or after an earlier error). Concurrent Get calls for the same profile
// block on a single loader invocation.
func (c *awsCache) Get(ctx context.Context, profile string) (aws.Config, error) {
	c.mu.Lock()
	if cfg, ok := c.configs[profile]; ok {
		c.mu.Unlock()
		return cfg, nil
	}
	lk, ok := c.locks[profile]
	if !ok {
		lk = &sync.Mutex{}
		c.locks[profile] = lk
	}
	c.mu.Unlock()

	lk.Lock()
	defer lk.Unlock()

	// Double-check after acquiring the per-profile lock: another goroutine
	// may have populated the cache while we were waiting.
	c.mu.Lock()
	if cfg, ok := c.configs[profile]; ok {
		c.mu.Unlock()
		return cfg, nil
	}
	c.mu.Unlock()

	cfg, err := c.loader(ctx, profile)
	if err != nil {
		return aws.Config{}, err
	}
	c.mu.Lock()
	c.configs[profile] = cfg
	c.mu.Unlock()
	return cfg, nil
}
