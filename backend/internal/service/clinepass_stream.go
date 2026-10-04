package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// ClinePass upstream only implements streaming generation: a request with
// stream=false is rejected with "generateText is not implemented". The gateway
// therefore always asks the upstream for a stream and -- when the client asked
// for a non-streaming response -- aggregates the SSE chunks back into a single
// Chat Completions JSON body.
//
// Aggregation keeps the client-visible contract identical to any other raw
// Chat Completions upstream: one `chat.completion` object with the full
// assistant message, tool calls and usage.

type clinePassStreamAccumulator struct {
	id      string
	created int64
	model   string

	content   strings.Builder
	reasoning strings.Builder
	refusal   strings.Builder

	toolCalls   map[int]*clinePassToolCall
	toolCallIDs []int

	finishReason string
	usage        OpenAIUsage
	hasUsage     bool

	sawData bool
}

type clinePassToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

func newClinePassStreamAccumulator() *clinePassStreamAccumulator {
	return &clinePassStreamAccumulator{toolCalls: map[int]*clinePassToolCall{}}
}

func (a *clinePassStreamAccumulator) observe(payload string) {
	if strings.TrimSpace(payload) == "" || strings.TrimSpace(payload) == "[DONE]" {
		return
	}
	a.sawData = true
	if id := gjson.Get(payload, "id").String(); id != "" {
		a.id = id
	}
	if created := gjson.Get(payload, "created"); created.Exists() {
		a.created = created.Int()
	}
	if model := gjson.Get(payload, "model").String(); model != "" {
		a.model = model
	}
	if u := extractCCStreamUsage(payload); u != nil {
		a.usage = *u
		a.hasUsage = true
	}
	choices := gjson.Get(payload, "choices")
	if !choices.Exists() || !choices.IsArray() {
		return
	}
	for _, choice := range choices.Array() {
		if reason := choice.Get("finish_reason").String(); reason != "" {
			a.finishReason = reason
		}
		delta := choice.Get("delta")
		if !delta.Exists() {
			delta = choice.Get("message")
		}
		if !delta.Exists() {
			continue
		}
		if content := delta.Get("content").String(); content != "" {
			a.content.WriteString(content)
		}
		if refusal := delta.Get("refusal").String(); refusal != "" {
			a.refusal.WriteString(refusal)
		}
		for _, key := range []string{"reasoning_content", "reasoning"} {
			if part := delta.Get(key).String(); part != "" {
				a.reasoning.WriteString(part)
				break
			}
		}
		if calls := delta.Get("tool_calls"); calls.Exists() && calls.IsArray() {
			for _, raw := range calls.Array() {
				index := int(raw.Get("index").Int())
				call, ok := a.toolCalls[index]
				if !ok {
					call = &clinePassToolCall{}
					a.toolCalls[index] = call
					a.toolCallIDs = append(a.toolCallIDs, index)
				}
				if id := raw.Get("id").String(); id != "" {
					call.id = id
				}
				if name := raw.Get("function.name").String(); name != "" {
					call.name = name
				}
				if args := raw.Get("function.arguments").String(); args != "" {
					call.arguments.WriteString(args)
				}
			}
		}
	}
}

// build assembles the aggregated Chat Completions response body. fromModel and
// toModel let the caller rewrite the upstream model back to the client-visible
// name, mirroring the streaming path.
func (a *clinePassStreamAccumulator) build(fromModel, toModel string) []byte {
	body := []byte(`{"object":"chat.completion","choices":[]}`)
	body, _ = sjson.SetBytes(body, "id", a.id)
	body, _ = sjson.SetBytes(body, "created", a.created)
	body, _ = sjson.SetBytes(body, "model", a.model)
	body, _ = sjson.SetBytes(body, "choices.0.index", 0)
	body, _ = sjson.SetBytes(body, "choices.0.message.role", "assistant")
	body, _ = sjson.SetBytes(body, "choices.0.message.content", a.content.String())
	body, _ = sjson.SetBytes(body, "choices.0.finish_reason", a.finishReason)
	if reasoning := a.reasoning.String(); reasoning != "" {
		body, _ = sjson.SetBytes(body, "choices.0.message.reasoning_content", reasoning)
	}
	if refusal := a.refusal.String(); refusal != "" {
		body, _ = sjson.SetBytes(body, "choices.0.message.refusal", refusal)
	}
	if len(a.toolCallIDs) > 0 {
		for position, index := range a.toolCallIDs {
			call := a.toolCalls[index]
			prefix := fmt.Sprintf("choices.0.message.tool_calls.%d.", position)
			body, _ = sjson.SetBytes(body, prefix+"index", index)
			body, _ = sjson.SetBytes(body, prefix+"type", "function")
			body, _ = sjson.SetBytes(body, prefix+"id", call.id)
			body, _ = sjson.SetBytes(body, prefix+"function.name", call.name)
			body, _ = sjson.SetBytes(body, prefix+"function.arguments", call.arguments.String())
		}
	}
	if a.hasUsage {
		usage, err := json.Marshal(a.usage)
		if err == nil {
			body, _ = sjson.SetRawBytes(body, "usage", usage)
		}
	}
	if fromModel != "" && toModel != "" && fromModel != toModel {
		body = []byte(strings.ReplaceAll(string(body), `"`+fromModel+`"`, `"`+toModel+`"`))
	}
	return body
}

// bufferClinePassStream drains the upstream SSE stream and writes a single
// non-streaming Chat Completions JSON response to the client.
func (s *OpenAIGatewayService) bufferClinePassStream(
	c *gin.Context,
	resp *http.Response,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
	requestBodyLen int,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	scanner := s.newUpstreamSSEScanner(resp.Body)
	accumulator := newClinePassStreamAccumulator()
	var firstTokenMs *int

	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "[DONE]" {
			continue
		}
		observer.ObserveOpenAI([]byte(payload), strings.TrimSpace(gjson.Get(payload, "type").String()))
		if firstTokenMs == nil && !isOpenAIChatUsageOnlyStreamChunk(payload) {
			elapsed := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &elapsed
		}
		accumulator.observe(payload)
	}

	scanErr := scanner.Err()
	if scanErr != nil && !errors.Is(scanErr, context.Canceled) && !errors.Is(scanErr, context.DeadlineExceeded) {
		logger.L().Warn("clinepass: stream read error while aggregating non-stream response",
			zap.Error(scanErr),
			zap.String("request_id", requestID),
			zap.Int64("account_id", account.ID),
		)
	}
	if !accumulator.sawData {
		return nil, newOpenAIUpstreamStreamReadError(scanErr)
	}

	body := accumulator.build(upstreamModel, originalModel)
	body = s.replaceModelInResponseBody(body, upstreamModel, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	if _, werr := c.Writer.Write(body); werr != nil {
		logger.L().Debug("clinepass: client disconnected while writing aggregated response",
			zap.Error(werr), zap.String("request_id", requestID))
	}

	return &OpenAIForwardResult{
		RequestID:                     requestID,
		UpstreamHeaders:               resp.Header,
		Usage:                         accumulator.usage,
		Model:                         originalModel,
		BillingModel:                  billingModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier:   observedUpstreamResponseServiceTier(c),
		ReasoningEffort:               reasoningEffort,
		ServiceTier:                   resolvedOpenAIUpstreamServiceTier(c, serviceTier),
		Stream:                        false,
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
	}, nil
}
