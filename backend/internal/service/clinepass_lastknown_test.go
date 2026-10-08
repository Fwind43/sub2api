//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeClinePassLastKnownStore is an in-memory ClinePassLastKnownStore used to
// verify the recording path without a database.
type fakeClinePassLastKnownStore struct {
	upserts []*ClinePassLastKnown
}

func (f *fakeClinePassLastKnownStore) UpsertClinePassLastKnown(_ context.Context, rec *ClinePassLastKnown) error {
	f.upserts = append(f.upserts, rec)
	return nil
}

func (f *fakeClinePassLastKnownStore) ListClinePassLastKnown(_ context.Context, _ int64) ([]ClinePassLastKnown, error) {
	out := make([]ClinePassLastKnown, 0, len(f.upserts))
	for _, r := range f.upserts {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeClinePassLastKnownStore) GetClinePassLastKnown(_ context.Context, accountID int64, model string) (*ClinePassLastKnown, error) {
	for _, r := range f.upserts {
		if r.AccountID == accountID && r.Model == model {
			return r, nil
		}
	}
	return nil, nil
}

// planner pipeline (Vercel AI Gateway): provider is reported under
// choices[0].message.provider_metadata.gateway.routing.finalProvider.
func TestParseClinePassRoutingPlanner(t *testing.T) {
	raw := `{"model":"z-ai/glm-5.3-flash","choices":[{"message":{"content":"hi","provider_metadata":{"gateway":{"routing":{"finalProvider":"deepinfra","canonicalSlug":"z-ai/glm-5.3-flash","planningReasoning":"chose deepinfra for cost","fallbacksAvailable":["novita","wafer"]}}}}}]}`
	got := ParseClinePassRouting([]byte(raw))
	require.NotNil(t, got)
	require.Equal(t, "planner", got.Pipeline)
	require.Equal(t, "deepinfra", got.FinalProvider)
	require.Equal(t, "deepinfra", got.FinalProviderName)
	require.Equal(t, "z-ai/glm-5.3-flash", got.CanonicalSlug)
	require.Equal(t, "chose deepinfra for cost", got.Plan)
	require.Equal(t, []string{"novita", "wafer"}, got.Fallbacks)
	require.Equal(t, "z-ai/glm-5.3-flash", got.Model)
}

// direct pipeline (OpenRouter): provider is reported at the top level.
func TestParseClinePassRoutingDirect(t *testing.T) {
	raw := `{"id":"gen-1","provider":"Deep Infra","model":"z-ai/glm-5.3-flash","choices":[{"message":{"content":"hi"}}]}`
	got := ParseClinePassRouting([]byte(raw))
	require.NotNil(t, got)
	require.Equal(t, "direct", got.Pipeline)
	require.Equal(t, "deep-infra", got.FinalProvider)
	require.Equal(t, "Deep Infra", got.FinalProviderName)
	require.Equal(t, "z-ai/glm-5.3-flash", got.CanonicalSlug)
}

// Streaming chunks are wrapped in a {"data": {...}} envelope.
func TestParseClinePassRoutingDataEnvelope(t *testing.T) {
	raw := `{"data":{"choices":[{"message":{"provider_metadata":{"gateway":{"routing":{"finalProvider":"openrouter","canonicalSlug":"z-ai/glm-5.3-flash"}}}}}]}}`
	got := ParseClinePassRouting([]byte(raw))
	require.NotNil(t, got)
	require.Equal(t, "planner", got.Pipeline)
	require.Equal(t, "openrouter", got.FinalProvider)
}

// No routing information -> nil, so callers never persist an empty observation.
func TestParseClinePassRoutingAbsent(t *testing.T) {
	require.Nil(t, ParseClinePassRouting(nil))
	require.Nil(t, ParseClinePassRouting([]byte("")))
	require.Nil(t, ParseClinePassRouting([]byte("   ")))
	require.Nil(t, ParseClinePassRouting([]byte("[DONE]")))
	require.Nil(t, ParseClinePassRouting([]byte("not json at all")))
	require.Nil(t, ParseClinePassRouting([]byte(`{"choices":[{"message":{"content":"hi"}}]}`)))
}

func TestClinePassSlugify(t *testing.T) {
	require.Equal(t, "deep-infra", clinePassSlugify("Deep Infra"))
	require.Equal(t, "novita", clinePassSlugify("  Novita  "))
	require.Equal(t, "", clinePassSlugify("   "))
}

// RecordClinePassRouting persists the observation with the effective model and
// falls back to the model carried inside the routing payload.
func TestRecordClinePassRoutingPersists(t *testing.T) {
	store := &fakeClinePassLastKnownStore{}
	svc := &OpenAIGatewayService{}
	svc.SetClinePassLastKnownStore(store)

	routing := &ClinePassRouting{Pipeline: "planner", FinalProvider: "deepinfra", CanonicalSlug: "z-ai/glm-5.3-flash", Model: "z-ai/glm-5.3-flash"}
	svc.RecordClinePassRouting(context.Background(), 1751, "glm-5.3-flash", routing)

	require.Len(t, store.upserts, 1)
	require.Equal(t, int64(1751), store.upserts[0].AccountID)
	require.Equal(t, "glm-5.3-flash", store.upserts[0].Model)
	require.Equal(t, "deepinfra", store.upserts[0].Provider)
	require.Equal(t, "planner", store.upserts[0].Pipeline)
	require.False(t, store.upserts[0].ObservedAt.IsZero())

	// Model falls back to routing.Model when the caller passes an empty model.
	svc.RecordClinePassRouting(context.Background(), 1751, "", routing)
	require.Len(t, store.upserts, 2)
	require.Equal(t, "z-ai/glm-5.3-flash", store.upserts[1].Model)

	// Nothing is recorded without an actual provider.
	svc.RecordClinePassRouting(context.Background(), 1751, "glm-5.3-flash", &ClinePassRouting{Pipeline: "planner"})
	require.Len(t, store.upserts, 2)

	// Nothing is recorded without a store (fail-open).
	noStore := &OpenAIGatewayService{}
	noStore.RecordClinePassRouting(context.Background(), 1751, "glm-5.3-flash", routing)
}
