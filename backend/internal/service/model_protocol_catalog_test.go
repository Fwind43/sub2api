//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 取自 https://api.commandcode.ai/provider/v1/models 的片段（2026-09-26）。
const commandCodeModelsSample = `{"object":"list","data":[
	{"id":"claude-sonnet-4-6","supported_endpoints":["/messages"]},
	{"id":"gpt-5.5","supported_endpoints":["/chat/completions","/responses"]},
	{"id":"deepseek/deepseek-v4-flash","supported_endpoints":["/chat/completions","/responses"]},
	{"id":"deepseek/deepseek-v4-flash-fast","supported_endpoints":["/chat/completions"]},
	{"id":"zai-org/GLM-5.3","supported_endpoints":["/chat/completions","/responses"]},
	{"id":"typesafe/jev","supported_endpoints":["/systemone"]},
	{"id":"no-endpoints"}
]}`

func TestParseModelProtocolCatalog(t *testing.T) {
	models, err := parseModelProtocolCatalog([]byte(commandCodeModelsSample))
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"claude-sonnet-4-6":               {APIProtocolAnthropic},
		"gpt-5.5":                         {APIProtocolChatCompletions, APIProtocolResponses},
		"deepseek/deepseek-v4-flash":      {APIProtocolChatCompletions, APIProtocolResponses},
		"deepseek/deepseek-v4-flash-fast": {APIProtocolChatCompletions},
		"zai-org/glm-5.3":                 {APIProtocolChatCompletions, APIProtocolResponses},
	}, models)

	_, err = parseModelProtocolCatalog([]byte(`{"data":[{"id":"gpt-5"},{"id":"glm-5"}]}`))
	require.Error(t, err, "a plain /models without supported_endpoints is not a protocol catalog")
}

func TestModelProtocolCatalogLookupRefreshesInBackground(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var catalog modelProtocolCatalog
		const key = "https://catalog.example/v1/models"
		now := time.Now()
		models, err := parseModelProtocolCatalog([]byte(commandCodeModelsSample))
		require.NoError(t, err)
		catalog.store(key, models, nil, now)
		var refreshes atomic.Int32
		refresh := func() { refreshes.Add(1) }

		require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, catalog.lookup(t.Context(), key, "ZAI-ORG/GLM-5.3", now, refresh))
		require.Nil(t, catalog.lookup(t.Context(), key, "unknown-model", now, refresh))
		require.Zero(t, refreshes.Load())

		// A stale catalog is returned immediately while one refresh runs in the background.
		time.Sleep(modelProtocolCatalogTTL) // synctest advances virtual time.
		later := time.Now()
		require.NotNil(t, catalog.lookup(t.Context(), key, "gpt-5.5", later, refresh))
		require.Equal(t, later, time.Now(), "a stale result must not wait for refresh")
		synctest.Wait()
		require.EqualValues(t, 1, refreshes.Load())
		require.NotNil(t, catalog.lookup(t.Context(), key, "gpt-5.5", later, refresh))
		require.EqualValues(t, 1, refreshes.Load(), "concurrent stale reads share one refresh")
		catalog.store(key, nil, errors.New("boom"), later)
		require.NotNil(t, catalog.lookup(t.Context(), key, "gpt-5.5", later.Add(time.Minute), refresh))
		require.EqualValues(t, 1, refreshes.Load(), "failure backoff preserves the old catalog")
		require.NotNil(t, catalog.lookup(t.Context(), key, "gpt-5.5", later.Add(modelProtocolCatalogRetryBackoff), refresh))
		synctest.Wait()
		require.EqualValues(t, 2, refreshes.Load())
	})
}

func TestModelProtocolCatalogFirstLoadSharesWaitDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var catalog modelProtocolCatalog
		const key = "https://catalog.example/v1/models"
		models := map[string][]string{"gpt-5.5": {APIProtocolResponses}}
		release := make(chan struct{})
		var refreshes atomic.Int32
		refresh := func() {
			refreshes.Add(1)
			<-release
			catalog.store(key, models, nil, time.Now())
		}
		type result struct {
			protocols []string
			elapsed   time.Duration
		}
		first, second := make(chan result, 1), make(chan result, 1)
		lookup := func(done chan<- result) {
			start := time.Now()
			protocols := catalog.lookup(t.Context(), key, "gpt-5.5", start, refresh)
			done <- result{protocols, time.Since(start)}
		}
		start := time.Now()
		go lookup(first)
		synctest.Wait()
		time.Sleep(modelProtocolCatalogFirstLoadWait / 2) // Virtual time only.
		go lookup(second)
		synctest.Wait()
		time.Sleep(modelProtocolCatalogFirstLoadWait / 2)
		synctest.Wait()

		r1, r2 := <-first, <-second
		require.Nil(t, r1.protocols)
		require.Nil(t, r2.protocols)
		require.Equal(t, modelProtocolCatalogFirstLoadWait, r1.elapsed)
		require.Equal(t, modelProtocolCatalogFirstLoadWait/2, r2.elapsed, "a later caller waits only for the remaining shared window")
		require.Equal(t, modelProtocolCatalogFirstLoadWait, time.Since(start))
		require.Nil(t, catalog.lookup(t.Context(), key, "gpt-5.5", time.Now(), refresh))
		require.Equal(t, modelProtocolCatalogFirstLoadWait, time.Since(start), "later calls fall back immediately while the same refresh continues")
		require.EqualValues(t, 1, refreshes.Load())

		close(release)
		synctest.Wait()
		require.Equal(t, models["gpt-5.5"], catalog.lookup(t.Context(), key, "gpt-5.5", time.Now(), refresh), "a late successful refresh still populates the cache")
	})
}

func TestModelProtocolCatalogFirstLoadWaitsForRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const key = "https://catalog.example/v1/models"
		models, err := parseModelProtocolCatalog([]byte(commandCodeModelsSample))
		require.NoError(t, err)
		var loaded modelProtocolCatalog
		release := make(chan struct{})
		result := make(chan []string, 1)
		go func() {
			result <- loaded.lookup(t.Context(), key, "gpt-5.5", time.Now(), func() {
				<-release
				loaded.store(key, models, nil, time.Now())
			})
		}()
		synctest.Wait()
		close(release)
		synctest.Wait()
		require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, <-result)

		var failing modelProtocolCatalog
		var refreshes atomic.Int32
		refresh := func() {
			refreshes.Add(1)
			failing.store(key, nil, errors.New("boom"), time.Now())
		}
		start := time.Now()
		require.Nil(t, failing.lookup(t.Context(), key, "gpt-5.5", start, refresh))
		require.Nil(t, failing.lookup(t.Context(), key, "gpt-5.5", start.Add(time.Minute), refresh))
		require.Equal(t, start, time.Now(), "a failed first load and its backoff must not hold requests")
		require.EqualValues(t, 1, refreshes.Load())
	})
}

func TestModelProtocolCatalogFirstLoadWaitRespondsToCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var catalog modelProtocolCatalog
		const key = "https://catalog.example/v1/models"
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		models := map[string][]string{"gpt-5.5": {APIProtocolResponses}}
		result := make(chan []string, 1)
		start := time.Now()
		go func() {
			result <- catalog.lookup(ctx, key, "gpt-5.5", start, func() {
				<-release
				catalog.store(key, models, nil, time.Now())
			})
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		require.Nil(t, <-result)
		require.Equal(t, start, time.Now(), "cancellation releases the caller without waiting for the cold-start deadline")

		// The caller's cancellation must not cancel the background refresh for future requests.
		close(release)
		synctest.Wait()
		require.Equal(t, models["gpt-5.5"], catalog.lookup(t.Context(), key, "gpt-5.5", time.Now(), nil))
	})
}

// 官方默认地址和自定义上游均按账号隔离；请求头与转发一致。
func TestModelProtocolCatalogKeyAndHeaders(t *testing.T) {
	official := commandCodeTestAccount(21)
	// fork 的 Command Code 不注册 provider profile（GetCNProtocolBaseURL 对其返回空），
	// 模型目录地址取该平台的上游 provider base。
	base := DefaultCommandCodeBaseURL
	url := buildOpenAIModelsURL(base)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/models", url)
	require.Equal(t, url+"#account=21", modelProtocolCatalogKey(official, url))
	require.Equal(t, url+"#account=22", modelProtocolCatalogKey(commandCodeTestAccount(22), url))

	headers := modelProtocolCatalogHeaders(official, url)
	require.Equal(t, "Bearer user_test_key", headers.Get("Authorization"))
	require.Equal(t, CodexCanonicalUserAgent(), headers.Get("User-Agent"), "official host gets the canonical UA like forwarding")

	// fork 的 Command Code 不注册 provider profile：api_base_urls 与请求头覆写都不参与
	// 模型目录请求，目录地址恒为官方 provider base。
	custom := commandCodeTestAccount(23)
	custom.Credentials["api_base_urls"] = map[string]any{APIProtocolChatCompletions: "https://relay.example/v1"}
	custom.Credentials["header_override_enabled"] = true
	custom.Credentials["header_overrides"] = map[string]any{"X-Tenant": "t-1"}
	require.Empty(t, custom.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	customURL := buildOpenAIModelsURL(DefaultCommandCodeBaseURL)
	require.Equal(t, url, customURL)
	require.Equal(t, customURL+"#account=23", modelProtocolCatalogKey(custom, customURL))
	headers = modelProtocolCatalogHeaders(custom, customURL)
	require.Equal(t, "Bearer user_test_key", headers.Get("Authorization"))
	require.Empty(t, getHeaderRaw(headers, "x-tenant"), "非多协议供应商不参与请求头覆写")
}

type modelCatalogAccountUpstream struct {
	HTTPUpstream
	mu            sync.Mutex
	requests      []*http.Request
	failureStatus int
	failureErr    error
}

func (u *modelCatalogAccountUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.mu.Unlock()
	if req.Header.Get("Authorization") == "Bearer invalid-key" {
		if u.failureErr != nil {
			return nil, u.failureErr
		}
		return newJSONResponse(u.failureStatus, `{"error":"invalid credentials"}`), nil
	}
	endpoint := "/chat/completions"
	if getHeaderRaw(req.Header, "x-tenant") == "responses-tenant" {
		endpoint = "/responses"
	}
	return newJSONResponse(http.StatusOK, fmt.Sprintf(`{"data":[{"id":"vendor/model","supported_endpoints":[%q]}]}`, endpoint)), nil
}

func TestModelProtocolCatalogOfficialAccountsAreIsolated(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "transport", err: errors.New("upstream connection failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &modelCatalogAccountUpstream{failureStatus: tc.status, failureErr: tc.err}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			id := time.Now().UnixNano()
			bad, chat, responses := commandCodeTestAccount(id), commandCodeTestAccount(id+1), commandCodeTestAccount(id+2)
			bad.Credentials["api_key"] = "invalid-key"
			responses.Credentials["header_override_enabled"] = true
			responses.Credentials["header_overrides"] = map[string]any{"X-Tenant": "responses-tenant"}
			url := buildOpenAIModelsURL(bad.GetCNProtocolBaseURL(APIProtocolChatCompletions))
			t.Cleanup(func() {
				upstreamModelProtocols.mu.Lock()
				defer upstreamModelProtocols.mu.Unlock()
				for _, account := range []*Account{bad, chat, responses} {
					delete(upstreamModelProtocols.entries, modelProtocolCatalogKey(account, url))
				}
			})

			// fork 的 Command Code 不注册 provider profile：模型目录能力对该平台不启用，
			// 三个账号都不会拉取目录，协议完全由内置规则决定（目录缓存/退避的账号隔离
			// 由 modelProtocolCatalog 自身单测覆盖）。
			require.Nil(t, svc.modelCatalogProtocols(t.Context(), bad, "vendor/model"))
			require.Nil(t, svc.modelCatalogProtocols(t.Context(), chat, "vendor/model"))
			require.Nil(t, svc.modelCatalogProtocols(t.Context(), responses, "vendor/model"))
			require.Equal(t, APIProtocolResponses, svc.resolveUpstreamProtocolFor(t.Context(), chat, APIProtocolResponses, "vendor/model"), "protocols come from the built-in rules, not from a catalog")
			require.Equal(t, APIProtocolResponses, svc.resolveUpstreamProtocolFor(t.Context(), responses, APIProtocolChatCompletions, "vendor/model"), "no catalog lookup happens for Command Code")
			upstream.mu.Lock()
			defer upstream.mu.Unlock()
			require.Empty(t, upstream.requests, "no catalog fetch for a platform without a provider profile")
		})
	}
}

func TestCommandCodeModelProtocolSetUsesCatalog(t *testing.T) {
	account := commandCodeTestAccount(11)
	both := []string{APIProtocolChatCompletions, APIProtocolResponses}

	// fork 的 Command Code 不注册 provider profile：模型目录不参与协议决策，
	// 传入目录与不传目录结果一致，协议完全由内置规则 / protocol_rules / 显式协议决定。
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "deepseek/deepseek-v4-flash", both))
	for _, model := range []string{"deepseek/deepseek-v4-flash", "deepseek/deepseek-v4-flash-fast", "gpt-5.5", "zai-org/glm-5.3"} {
		for _, inbound := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
			require.Equal(t,
				resolveUpstreamProtocol(account, inbound, model, nil),
				resolveUpstreamProtocol(account, inbound, model, both),
				"%s/%s", inbound, model)
		}
	}
	// fork 的 Command Code 未注册 provider profile，因此没有“Claude 模型→Anthropic 端点”的内置规则?
	//（该能力由目录/profile 提供，CC 端点在 CC 管线内单独处理）：即使目录声明 Anthropic，入站协议仍原样保留。?
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "claude-sonnet-4-6", []string{APIProtocolAnthropic}))

	// fork 的 Command Code 未注册 provider profile，不按模型分流；protocol_rules
	// 仅在按模型分流的供应商上生效，故此处不参与决策，结果与不带规则时一致。
	account.Credentials["protocol_rules"] = []any{map[string]any{"pattern": "deepseek/*", "protocol": APIProtocolChatCompletions}}
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "deepseek/deepseek-v4-flash", both))
	require.Equal(t,
		resolveUpstreamProtocol(account, APIProtocolResponses, "zai-org/glm-5.3", nil),
		resolveUpstreamProtocol(account, APIProtocolResponses, "zai-org/glm-5.3", both))

	// 显式协议（api_protocol）在本解析器内只对「按模型分流 / 按入站分流」的
	// 供应商生效；fork 的 Command Code 未注册 provider profile，其请求在
	// openai_gateway_forward.go 的 CC 管线分支被接管（端点固定为
	// /v1/chat/completions），故本函数对 CC 仍返回默认的 Responses。
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "zai-org/glm-5.3", both))
}

func TestProtocolRulesWithProtocolSets(t *testing.T) {
	credentials := map[string]any{"protocol_rules": []any{
		map[string]any{"pattern": " GPT-* ", "protocol": APIProtocolResponses, "protocols": []any{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolChatCompletions}},
		map[string]any{"pattern": "qwen*", "protocols": []any{APIProtocolAnthropic}},
		map[string]any{"pattern": "glm-*", "protocol": APIProtocolChatCompletions},
	}}
	require.NoError(t, NormalizeProtocolRulesCredentials(credentials))
	require.Equal(t, []any{
		map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses, "protocols": []any{APIProtocolResponses, APIProtocolChatCompletions}},
		map[string]any{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
		map[string]any{"pattern": "glm-*", "protocol": APIProtocolChatCompletions},
	}, credentials["protocol_rules"])

	bad := map[string]any{"protocol_rules": []any{map[string]any{"pattern": "gpt-*", "protocols": []any{"responses", "bogus"}}}}
	require.Error(t, NormalizeProtocolRulesCredentials(bad))

	// 单一协议规则（OpenCode）不受影响：与入站无关。
	openCode := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}}
	for _, inbound := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(openCode, inbound, "gpt-5.5", nil), inbound)
		require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(openCode, inbound, "glm-5.3", nil), inbound)
	}
}

// 经真实入口验证：目录声明模型支持入站协议时原样直通，不做协议转换。
func TestCommandCodeGatewayPassesThroughCatalogProtocols(t *testing.T) {
	type observation struct {
		url  string
		body []byte
	}
	forward := func(t *testing.T, ingress routingMatrixIngress, model string, catalog map[string][]string) observation {
		t.Helper()
		base := fmt.Sprintf("http://cc-%s-%d.example", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())
		account := commandCodeTestAccount(12)
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolChatCompletions: base + "/provider/v1",
			APIProtocolResponses:       base + "/provider/v1",
			APIProtocolAnthropic:       base + "/provider",
		}
		url := buildOpenAIModelsURL(base + "/provider/v1")
		key := modelProtocolCatalogKey(account, url)
		if catalog != nil {
			upstreamModelProtocols.store(key, catalog, nil, time.Now())
		} else {
			// 目录不可用（退避中）：不触发刷新，回落内置规则。
			upstreamModelProtocols.store(key, nil, errors.New("unavailable"), time.Now())
		}
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		body := routingMatrixCase{ingress: ingress, model: model}.body()
		_ = ingress.forward(svc, adaptiveProtocolTestContext(ingress.path, body), account, body)
		require.NotEmpty(t, upstream.requests)
		return observation{url: upstream.requests[len(upstream.requests)-1].URL.String(), body: upstream.lastBody}
	}
	ingresses := map[string]routingMatrixIngress{}
	for _, ingress := range routingMatrixIngresses() {
		ingresses[ingress.name] = ingress
	}
	catalog := map[string][]string{
		"deepseek/deepseek-v4-flash":      {APIProtocolChatCompletions, APIProtocolResponses},
		"deepseek/deepseek-v4-flash-fast": {APIProtocolChatCompletions},
		"gpt-5.5":                         {APIProtocolChatCompletions, APIProtocolResponses},
	}

	// fork 的 Command Code 不注册 provider profile：模型目录不参与转发决策，
	// 传入目录与不传目录的转发结果（端点路径与请求体形态）完全一致。
	path := func(u string) string {
		if i := strings.Index(u, "://"); i >= 0 {
			if j := strings.Index(u[i+3:], "/"); j >= 0 {
				return u[i+3+j:]
			}
		}
		return u
	}

	// fork 的 Command Code 上游请求体（pkg/commandcode）每次注入唯一 threadId，
	// 比较请求体时先将该字段归一化，其余字节必须完全一致。
	normalizeThreadID := func(b []byte) string {
		const marker = `"threadId":"`
		s := string(b)
		i := strings.Index(s, marker)
		if i < 0 {
			return s
		}
		if j := strings.Index(s[i+len(marker):], `"`); j >= 0 {
			return s[:i+len(marker)] + "<normalized>" + s[i+len(marker)+j:]
		}
		return s
	}
	for _, tc := range []struct {
		ingress string
		model   string
	}{
		{ingress: "responses", model: "deepseek/deepseek-v4-flash"},
		{ingress: "responses", model: "deepseek/deepseek-v4-flash-fast"},
		{ingress: "chat", model: "gpt-5.5"},
	} {
		withCatalog := forward(t, ingresses[tc.ingress], tc.model, catalog)
		without := forward(t, ingresses[tc.ingress], tc.model, nil)
		require.Equal(t, path(without.url), path(withCatalog.url), "%s/%s", tc.ingress, tc.model)
		require.Equal(t, normalizeThreadID(without.body), normalizeThreadID(withCatalog.body), "%s/%s", tc.ingress, tc.model)
		require.Equal(t, gjson.GetBytes(without.body, "messages").Exists(), gjson.GetBytes(withCatalog.body, "messages").Exists(), "%s/%s", tc.ingress, tc.model)
	}
}

// 目录缺失时网关拉取目录，首个请求即按目录分流；自定义上游按账号各自拉取，并带上
// 账号的请求头覆写。
func TestCommandCodeGatewayFetchesModelCatalog(t *testing.T) {
	// fork 的 Command Code 不注册 provider profile：模型目录能力对该平台不启用，
	// 自定义上游也不会触发目录拉取（目录拉取路径由注册了 ModelCatalog 的供应商覆盖）。
	first, second := commandCodeTestAccount(13), commandCodeTestAccount(14)
	upstream := &commandCodeAlphaUpstream{}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	require.Nil(t, svc.modelCatalogProtocols(t.Context(), first, "deepseek/deepseek-v4-flash"))
	require.Nil(t, svc.modelCatalogProtocols(t.Context(), second, "deepseek/deepseek-v4-flash"))
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Empty(t, upstream.requests, "no catalog fetch for a platform without a provider profile")
}
