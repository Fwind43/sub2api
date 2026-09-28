package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type commandCodeNativeCreateRepo struct {
	*upstreamBillingProbeAccountRepo
	saved []*Account
}

func (r *commandCodeNativeCreateRepo) Create(_ context.Context, account *Account) error {
	account.ID = int64(len(r.saved) + 1)
	stored := *account
	stored.Credentials = make(map[string]any, len(account.Credentials))
	for k, v := range account.Credentials {
		stored.Credentials[k] = v
	}
	r.saved = append(r.saved, &stored)
	return nil
}

func TestCommandCodeNativeAdminCreateAccounts(t *testing.T) {
	repo := &commandCodeNativeCreateRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{}}
	svc := &adminServiceImpl{accountRepo: repo}
	keys := []string{"native-fixture-key-one", "native-fixture-key-two"}
	for i, key := range keys {
		created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
			Name:                 fmt.Sprintf("native-commandcode-%d", i),
			Platform:             PlatformCommandCode,
			Type:                 AccountTypeAPIKey,
			Credentials:          map[string]any{"api_key": key},
			SkipDefaultGroupBind: true,
		})
		require.NoError(t, err)
		require.Equal(t, int64(i+1), created.ID)
		require.Equal(t, PlatformCommandCode, created.Platform)
		require.Equal(t, AccountTypeAPIKey, created.Type)
		require.Equal(t, key, created.Credentials["api_key"])
	}
	require.Len(t, repo.saved, 2)
	require.NotEqual(t, repo.saved[0].ID, repo.saved[1].ID)
	for i, stored := range repo.saved {
		require.Equal(t, PlatformCommandCode, stored.Platform)
		require.Equal(t, keys[i], stored.Credentials["api_key"])
		require.Equal(t, fmt.Sprintf("native-commandcode-%d", i), stored.Name)
	}
}
