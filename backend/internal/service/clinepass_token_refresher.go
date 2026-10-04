package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ClinePass access tokens are short-lived WorkOS tokens (1h default). Warm the
// pool by refreshing a little before expiry so request-path cache misses never
// hit an expired credential.
const clinePassTokenRefreshSkew = 30 * time.Minute

// ClinePassTokenCacheKey builds the distributed-lock cache key for an account.
func ClinePassTokenCacheKey(account *Account) string {
	if account == nil {
		return "clinepass:account:0"
	}
	return "clinepass:account:" + strconv.FormatInt(account.ID, 10)
}

// ClinePassTokenRefresher refreshes ClinePass OAuth accounts.
type ClinePassTokenRefresher struct {
	clinePassOAuthService *ClinePassOAuthService
}

func NewClinePassTokenRefresher(clinePassOAuthService *ClinePassOAuthService) *ClinePassTokenRefresher {
	return &ClinePassTokenRefresher{clinePassOAuthService: clinePassOAuthService}
}

func (r *ClinePassTokenRefresher) CacheKey(account *Account) string {
	return ClinePassTokenCacheKey(account)
}

func (r *ClinePassTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformClinePass && account.IsOAuth() &&
		strings.TrimSpace(account.GetClinePassRefreshToken()) != ""
}

func (r *ClinePassTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.GetClinePassRefreshToken()) == "" {
		return false
	}
	if strings.TrimSpace(account.GetClinePassAccessToken()) == "" {
		return true
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < clinePassTokenRefreshSkew {
		refreshWindow = clinePassTokenRefreshSkew
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *ClinePassTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.clinePassOAuthService == nil {
		return nil, errors.New("clinepass oauth service is not configured")
	}
	tokenInfo, err := r.clinePassOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCredentials := r.clinePassOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
	}
	return newCredentials, nil
}
