package commandcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// observingDoer preserves upstream status/headers before committing a stream.
// No account credentials or mutable state are shared between calls.
type observingDoer struct {
	inner HTTPDoer
	ready chan *http.Response
	once  sync.Once
}

func (d *observingDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.inner.Do(req)
	if err != nil {
		d.once.Do(func() { d.ready <- nil })
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			d.once.Do(func() { d.ready <- nil })
			return nil, readErr
		}
		snapshot := *resp
		snapshot.Body = io.NopCloser(bytes.NewReader(body))
		resp.Body = io.NopCloser(bytes.NewReader(body))
		d.once.Do(func() { d.ready <- &snapshot })
	} else {
		d.once.Do(func() { d.ready <- resp })
	}
	return resp, nil
}

func openAIUsage(raw map[string]any) map[string]any {
	number := func(keys ...string) int64 {
		for _, k := range keys {
			switch v := raw[k].(type) {
			case float64:
				return int64(v)
			case int:
				return int64(v)
			case int64:
				return v
			case map[string]any:
				if n, ok := v["total"].(float64); ok {
					return int64(n)
				}
			}
		}
		return 0
	}
	input := number("inputTokens", "input_tokens", "prompt_tokens")
	output := number("outputTokens", "output_tokens", "completion_tokens")
	return map[string]any{"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output}
}

// ChatCompletion adapts GO directly to the existing OpenAI-compatible pipeline.
// Caller cancellation and closing the response body both stop the producer.
func (c *Client) ChatCompletion(ctx context.Context, key string, body []byte, stream bool) (*http.Response, error) {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid CommandCode request: %w", err)
	}
	if c.HTTPClient == nil {
		return nil, fmt.Errorf("CommandCode HTTP client is required")
	}
	ctx, cancel := context.WithCancel(ctx)
	observer := &observingDoer{inner: c.HTTPClient, ready: make(chan *http.Response, 1)}
	client := *c
	client.HTTPClient = observer
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	id := fmt.Sprintf("chatcmpl-cc-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	go func() {
		defer cancel()
		emit := func(delta map[string]any, finish any, usage map[string]any) error {
			event := map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
			if usage != nil {
				event["usage"] = usage
			}
			data, err := json.Marshal(event)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(writer, "data: %s\n\n", data)
			return err
		}
		var onDelta func(string) error
		if stream {
			onDelta = func(text string) error { return emit(map[string]any{"role": "assistant", "content": text}, nil, nil) }
			req.OnReasoning = func(text string) error { return emit(map[string]any{"reasoning_content": text}, nil, nil) }
			req.OnToolCall = func(call map[string]any) error { return emit(map[string]any{"tool_calls": []any{call}}, nil, nil) }
		}
		result, err := client.Generate(ctx, key, req.Model, req, onDelta)
		observer.once.Do(func() { observer.ready <- nil })
		if err == nil {
			finish := result.FinishReason
			if finish == "" {
				finish = "stop"
			}
			usage := openAIUsage(result.Usage)
			if stream {
				err = emit(map[string]any{}, finish, usage)
				if err == nil {
					_, err = io.WriteString(writer, "data: [DONE]\n\n")
				}
			} else {
				message := map[string]any{"role": "assistant", "content": result.Text}
				if result.Reasoning != "" {
					message["reasoning_content"] = result.Reasoning
				}
				if len(result.ToolCalls) > 0 {
					message["tool_calls"] = result.ToolCalls
				}
				err = json.NewEncoder(writer).Encode(map[string]any{"id": id, "object": "chat.completion", "created": created, "model": req.Model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": usage})
			}
		}
		writer.CloseWithError(err)
		done <- err
	}()
	select {
	case upstream := <-observer.ready:
		if upstream == nil {
			reader.Close()
			cancel()
			return nil, <-done
		}
		if upstream.StatusCode != http.StatusOK {
			reader.Close()
			cancel()
			<-done
			return upstream, nil
		}
		header := upstream.Header.Clone()
		header.Del("Content-Length")
		header.Del("Content-Encoding")
		if stream {
			header.Set("Content-Type", "text/event-stream")
		} else {
			header.Set("Content-Type", "application/json")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: &cancelReadCloser{ReadCloser: reader, cancel: cancel}, ContentLength: -1}, nil
	case <-ctx.Done():
		reader.Close()
		cancel()
		return nil, ctx.Err()
	}
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error { r.cancel(); return r.ReadCloser.Close() }
