package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestAWSCacheMemoizesPerProfile(t *testing.T) {
	c := newAWSCache()
	var calls int32
	c.loader = func(_ context.Context, profile string) (aws.Config, error) {
		atomic.AddInt32(&calls, 1)
		return aws.Config{Region: profile + "-region"}, nil
	}

	cfg1, err := c.Get(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := c.Get(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if cfg1.Region != cfg2.Region {
		t.Errorf("expected same config, got %q vs %q", cfg1.Region, cfg2.Region)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("loader called %d times, want 1", got)
	}

	// Different profile → new load.
	if _, err := c.Get(context.Background(), "prod"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("loader called %d times, want 2", got)
	}
}

func TestAWSCacheSerializesPerProfileLoads(t *testing.T) {
	c := newAWSCache()
	var calls int32
	// Slow loader: if Get allowed two concurrent loads, we'd see >1 call.
	c.loader = func(_ context.Context, profile string) (aws.Config, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		return aws.Config{Region: profile}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), "dev"); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("loader called %d times for same profile, want 1 (singleflight)", got)
	}
}

func TestAWSCacheDoesNotCacheErrors(t *testing.T) {
	c := newAWSCache()
	var calls int32
	c.loader = func(_ context.Context, _ string) (aws.Config, error) {
		atomic.AddInt32(&calls, 1)
		return aws.Config{}, errors.New("transient")
	}

	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), "dev"); err == nil {
			t.Fatal("expected error")
		}
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("loader called %d times, want 3 (error should not be cached)", got)
	}
}
