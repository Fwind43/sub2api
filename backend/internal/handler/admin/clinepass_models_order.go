package admin

import (
	"sort"
	"strings"
)

// clinePassModelTier ranks ClinePass catalog entries for display order.
//
// The upstream recommended-model catalog mixes three families:
//
//   - cline-pass/*  : covered by the Cline subscription (works out of the box)
//   - anthropic/*   : premium, needs a positive Cline Credits balance
//   - cline-free/*  : rejected with 403 "only available via Cline product
//     surfaces" when called through the API relay
//
// The account-test modal preselects the first entry, so ordering the
// subscription family first keeps "Test connection" working for an
// out-of-the-box account instead of failing with a 402/403.
func clinePassModelTier(id string) int {
	switch {
	case strings.HasPrefix(id, "cline-pass/"):
		return 0
	case strings.HasPrefix(id, "cline-free/"):
		return 2
	default:
		return 1
	}
}

// sortClinePassModelIDs orders ClinePass model ids by tier, then
// alphabetically inside each tier.
func sortClinePassModelIDs(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		ti, tj := clinePassModelTier(ids[i]), clinePassModelTier(ids[j])
		if ti != tj {
			return ti < tj
		}
		return ids[i] < ids[j]
	})
}
