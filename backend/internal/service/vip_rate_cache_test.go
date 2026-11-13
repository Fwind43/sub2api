package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type versionedRateRepoStub struct {
	UserGroupRateRepository
	mu      sync.Mutex
	version uint64
	rate    float64
	calls   int
	started chan struct{}
	release chan struct{}
}

func (r *versionedRateRepoStub) UserGroupRateCacheVersion(_, _ int64) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version
}

func (r *versionedRateRepoStub) save(rate float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rate = rate
	r.version++
}

func (r *versionedRateRepoStub) GetByUserAndGroup(ctx context.Context, _, _ int64) (*float64, error) {
	r.mu.Lock()
	r.calls++
	rate := r.rate
	block := r.calls == 1 && r.started != nil
	r.mu.Unlock()
	if block {
		close(r.started)
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &rate, nil
}

func TestVIPCacheVersionRefreshesBothGateways(t *testing.T) {
	repo := &versionedRateRepoStub{rate: 0.8}
	gateway := &GatewayService{userGroupRateResolver: newUserGroupRateResolver(repo, nil, time.Hour, nil, "")}
	openai := &OpenAIGatewayService{userGroupRateResolver: newUserGroupRateResolver(repo, nil, time.Hour, nil, "")}
	ctx := context.Background()
	for _, rate := range []float64{0.8, 0.5, 1.0} {
		if rate != 0.8 {
			repo.save(rate)
		}
		require.Equal(t, rate, gateway.ResolveUserGroupRateMultiplier(ctx, 11, 21, 1.0))
		require.Equal(t, rate, openai.ResolveUserGroupRateMultiplier(ctx, 11, 21, 1.0))
		// Repeat without saving: both resolvers should retain their cache hits.
		require.Equal(t, rate, gateway.ResolveUserGroupRateMultiplier(ctx, 11, 21, 1.0))
		require.Equal(t, rate, openai.ResolveUserGroupRateMultiplier(ctx, 11, 21, 1.0))
	}
	require.Equal(t, 6, repo.calls)
}

func TestVIPCacheVersionIsolatesInflightLoadAndRefill(t *testing.T) {
	repo := &versionedRateRepoStub{rate: 0.8, started: make(chan struct{}), release: make(chan struct{})}
	resolver := newUserGroupRateResolver(repo, nil, time.Hour, nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var release sync.Once
	defer release.Do(func() { close(repo.release) })
	oldResult := make(chan float64, 1)
	go func() { oldResult <- resolver.Resolve(ctx, 11, 21, 1.0) }()
	select {
	case <-repo.started:
	case <-ctx.Done():
		t.Fatal("old query did not start")
	}

	repo.save(0.5)
	newResult := make(chan float64, 1)
	go func() { newResult <- resolver.Resolve(ctx, 11, 21, 1.0) }()
	select {
	case rate := <-newResult:
		require.Equal(t, 0.5, rate)
	case <-ctx.Done():
		t.Fatal("post-save request joined the old singleflight")
	}

	// Finish the old query after the new query has populated the new cache.
	release.Do(func() { close(repo.release) })
	select {
	case rate := <-oldResult:
		require.Equal(t, 0.8, rate)
	case <-ctx.Done():
		t.Fatal("old query did not complete")
	}
	require.Equal(t, 0.5, resolver.Resolve(ctx, 11, 21, 1.0))
	require.Equal(t, 2, repo.calls)
}
