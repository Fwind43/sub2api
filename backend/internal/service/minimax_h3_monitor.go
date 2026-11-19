package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type H3NodeHealth struct {
	OK       *bool  `json:"ok"`
	Upstream *bool  `json:"upstream"`
	GPU      string `json:"gpu"`
	Active   *int   `json:"active_jobs"`
	Capacity *int   `json:"max_queue"`
}
type H3NodeStatus struct {
	ID      int64         `json:"id"`
	Name    string        `json:"name"`
	Pending int           `json:"pending"`
	Queued  int           `json:"queued"`
	Health  *H3NodeHealth `json:"health"`
}

func (s *H3Service) NodeStatus(ctx context.Context) ([]H3NodeStatus, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,count(t.id),count(t.queue_position) FROM accounts a LEFT JOIN minimax_h3_tasks t ON t.account_id=a.id AND t.generation IN ('submitting','pending') WHERE a.deleted_at IS NULL AND a.platform='minimax' AND a.credentials->>'account_mode'='h3' GROUP BY a.id,a.name ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []H3NodeStatus{}
	for rows.Next() {
		var n H3NodeStatus
		if err := rows.Scan(&n.ID, &n.Name, &n.Pending, &n.Queued); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range nodes {
		nodes[i].Health = s.probeH3Node(ctx, nodes[i].ID)
	}
	return nodes, nil
}
func (s *H3Service) probeH3Node(ctx context.Context, id int64) *H3NodeHealth {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	a, err := s.gateway.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil
	}
	base, err := url.Parse(strings.TrimSpace(a.GetCredential("base_url")))
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil
	}
	base.Path = strings.TrimSuffix(strings.TrimRight(base.Path, "/"), "/v1") + "/healthz"
	base.RawPath = ""
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, base.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+a.GetCredential("api_key"))
	proxy := ""
	if a.Proxy != nil {
		proxy = a.Proxy.URL()
	}
	res, err := s.gateway.httpUpstream.Do(req, proxy, a.ID, a.Concurrency)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil
	}
	var health H3NodeHealth
	if err := json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&health); err != nil || health.OK == nil {
		return nil
	}
	return &health
}
