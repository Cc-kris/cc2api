package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

// Guard only the uncommitted prefix. After semantic output the original stream
// processors retain ownership, including their continuation and billing logic.
const maxFirstOutputPrefixBytes = 8 << 20

type replayStreamBody struct {
	io.Reader
	io.Closer
}

var errStreamPrefixIdleTimeout = errors.New("stream data interval timeout before output")

type prefixTerminalErrorReader struct{ err error }

func (r prefixTerminalErrorReader) Read([]byte) (int, error) { return 0, r.err }

type prefixMappedErrorBody struct {
	io.ReadCloser
	err error
}

func (b *prefixMappedErrorBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		err = b.err
	}
	return n, err
}

type streamPrefixResult struct {
	body    io.ReadCloser
	status  int
	payload []byte
	reason  string
	err     error
}

func streamSemanticFailure(payload []byte) int {
	code := strings.ToLower(gjson.GetBytes(payload, "response.error.code").String())
	if code == "" {
		code = strings.ToLower(gjson.GetBytes(payload, "error.code").String())
	}
	typ := strings.ToLower(gjson.GetBytes(payload, "response.error.type").String())
	if typ == "" {
		typ = strings.ToLower(gjson.GetBytes(payload, "error.type").String())
	}
	var status int64
	for _, path := range []string{"response.error.status_code", "response.error.status", "error.status_code", "error.status", "status_code", "status"} {
		if value := gjson.GetBytes(payload, path).Int(); value != 0 {
			status = value
			break
		}
	}
	// Preserve the legacy 502 classification for message-only capacity hints.
	// Only explicit status/code/type evidence is normalized to overload 529.
	if status == 529 {
		return 529
	}
	if status == 429 || code == "429" || typ == "rate_limit_error" || code == "rate_limit_exceeded" || code == "too_many_requests" {
		return http.StatusTooManyRequests
	}
	switch code {
	case "server_is_overloaded", "slow_down", "capacity_exceeded", "overloaded":
		return 529
	}
	if typ == "overloaded_error" {
		return 529
	}
	return 0
}

func responsesCompletedIsEmpty(payload []byte) bool {
	response := gjson.GetBytes(payload, "response")
	if !response.Exists() {
		response = gjson.ParseBytes(payload)
	}
	if response.Get("error").Exists() && response.Get("error").Type != gjson.Null {
		return false
	}
	if response.Get("usage.input_tokens").Int() > 0 || response.Get("usage.output_tokens").Int() > 0 {
		return false
	}
	meaningful := false
	response.Get("output").ForEach(func(_, item gjson.Result) bool {
		switch item.Get("type").String() {
		case "function_call", "custom_tool_call", "tool_call", "mcp_tool_call", "image_generation_call":
			meaningful = true
			return false
		}
		item.Get("content").ForEach(func(_, part gjson.Result) bool {
			if strings.TrimSpace(part.Get("text").String()) != "" || part.Get("refusal").String() != "" {
				meaningful = true
				return false
			}
			return true
		})
		return !meaningful
	})
	return !meaningful
}

func streamDataIsSemanticOutput(payload []byte, event string, anthropic bool) bool {
	if anthropic {
		if event == "message_stop" {
			return true
		}
		if event == "content_block_start" {
			block := gjson.GetBytes(payload, "content_block")
			return block.Get("type").String() == "tool_use" || block.Get("text").String() != "" || block.Get("thinking").String() != ""
		}
		if event == "content_block_delta" {
			delta := gjson.GetBytes(payload, "delta")
			return delta.Get("text").String() != "" || delta.Get("thinking").String() != "" || delta.Get("partial_json").String() != ""
		}
		return false
	}
	switch event {
	case "response.completed", "response.incomplete":
		return true
	case "response.created", "response.in_progress", "response.queued", "ping", "response.output_item.added", "response.content_part.added":
		item := gjson.GetBytes(payload, "item")
		switch item.Get("type").String() {
		case "image_generation_call":
			return item.Get("id").String() != ""
		case "function_call", "custom_tool_call", "mcp_tool_call", "tool_call":
			return item.Get("name").String() != "" || item.Get("call_id").String() != ""
		}
		return false
	default:
		if strings.HasSuffix(event, ".delta") {
			return gjson.GetBytes(payload, "delta").String() != ""
		}
		// Unknown protocol events are replayed immediately, rather than being lost.
		return event != "" && event != "response.failed" && event != "error"
	}
}

func readStreamPrefix(body io.ReadCloser, anthropic bool) streamPrefixResult {
	reader := bufio.NewReaderSize(body, 32<<10)
	var prefix bytes.Buffer
	var data []string
	event := ""
	replay := func() streamPrefixResult {
		return streamPrefixResult{body: &replayStreamBody{Reader: io.MultiReader(bytes.NewReader(prefix.Bytes()), reader), Closer: body}}
	}
	for {
		fragment, readErr := reader.ReadSlice('\n')
		if prefix.Len()+len(fragment) > maxFirstOutputPrefixBytes {
			return streamPrefixResult{status: 502, reason: "upstream metadata prefix exceeded safety limit"}
		}
		prefix.Write(fragment)
		// ReadSlice bounds individual allocations too; assemble only one SSE line.
		line := append([]byte(nil), fragment...)
		for readErr == bufio.ErrBufferFull {
			fragment, readErr = reader.ReadSlice('\n')
			if prefix.Len()+len(fragment) > maxFirstOutputPrefixBytes {
				return streamPrefixResult{status: 502, reason: "upstream metadata prefix exceeded safety limit"}
			}
			prefix.Write(fragment)
			line = append(line, fragment...)
		}
		text := strings.TrimRight(string(line), "\r\n")
		if strings.HasPrefix(text, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(text, "event:"))
		}
		if strings.HasPrefix(text, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(text, "data:")))
		}
		if text == "" && len(data) > 0 {
			payload := []byte(strings.Join(data, "\n"))
			data = nil
			if string(payload) == "[DONE]" {
				return replay()
			}
			kind := gjson.GetBytes(payload, "type").String()
			if kind == "" {
				kind = event
			}
			if kind == "error" || kind == "response.failed" {
				if status := streamSemanticFailure(payload); status != 0 {
					return streamPrefixResult{status: status, payload: payload, reason: "upstream capacity or rate-limit failure before output"}
				}
				if !anthropic && openAIStreamFailedEventShouldFailover(payload, extractUpstreamErrorMessage(payload)) {
					return streamPrefixResult{status: 502, payload: payload, reason: "upstream stream failed before output"}
				}
				return replay()
			}
			if !anthropic && kind == "response.completed" && responsesCompletedIsEmpty(payload) {
				return streamPrefixResult{status: 502, payload: payload, reason: "OpenAI Responses completed without output or usage"}
			}
			if streamDataIsSemanticOutput(payload, kind, anthropic) {
				return replay()
			}
			event = ""
		}
		if readErr != nil {
			if readErr == io.EOF {
				// Preserve the protocol processor's existing incomplete-stream diagnosis
				// and cache policy. EOF itself is not a new semantic capacity failure.
				return replay()
			}
			// Preserve the original parser's disconnect retry, address sanitization,
			// usage draining and cancellation semantics. Replay even metadata already
			// read before the transport error, then deliver that exact read error.
			return streamPrefixResult{body: &replayStreamBody{Reader: io.MultiReader(bytes.NewReader(prefix.Bytes()), prefixTerminalErrorReader{readErr}), Closer: body}}
		}
	}
}

// Prefix reads must keep the original idle watchdog alive even when the
// optional absolute first-output deadline is disabled. Metadata refreshes idle
// activity, but never extends the absolute deadline.
type prefixActivityBody struct {
	io.ReadCloser
	lastRead    atomic.Int64
	partial     atomic.Bool
	lineHasData bool // accessed only by the prefix-reading goroutine
}

func (b *prefixActivityBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.lastRead.Store(time.Now().UnixNano())
		for _, c := range p[:n] {
			switch c {
			case '\r':
			case '\n':
				if !b.lineHasData {
					b.partial.Store(false)
				}
				b.lineHasData = false
			default:
				b.lineHasData = true
				b.partial.Store(true)
			}
		}
	}
	return n, err
}

func guardStreamFirstOutput(ctx context.Context, resp *http.Response, timeout time.Duration, anthropic bool, idleTimeout ...time.Duration) streamPrefixResult {
	var idle time.Duration
	if len(idleTimeout) > 0 {
		idle = idleTimeout[0]
	}
	return guardStreamFirstOutputWithKeepalive(ctx, resp, timeout, anthropic, idle, 0, nil)
}

func guardStreamFirstOutputWithKeepalive(ctx context.Context, resp *http.Response, timeout time.Duration, anthropic bool, idle, keepalive time.Duration, ping func() error) streamPrefixResult {
	original := resp.Body
	observed := &prefixActivityBody{ReadCloser: original}
	observed.lastRead.Store(time.Now().UnixNano())
	var heartbeat <-chan time.Time
	var ticker *time.Ticker
	if keepalive > 0 && ping != nil {
		ticker = time.NewTicker(keepalive)
		heartbeat = ticker.C
		defer ticker.Stop()
	}
	var idleDeadline <-chan time.Time
	var idleTimer *time.Timer
	if idle > 0 {
		idleTimer = time.NewTimer(idle)
		idleDeadline = idleTimer.C
		defer idleTimer.Stop()
	}
	result := make(chan streamPrefixResult, 1)
	go func() { result <- readStreamPrefix(observed, anthropic) }()
	var deadline <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		deadline = timer.C
		defer timer.Stop()
	}
	// Close unblocks the prefix reader; join it before handing its buffered
	// bytes back to the protocol processor. Only one goroutine ever reads body.
	handoffReadError := func(err error) streamPrefixResult {
		_ = original.Close()
		r := <-result
		if r.body == nil {
			r.body = &replayStreamBody{Reader: prefixTerminalErrorReader{err}, Closer: original}
		} else {
			r.body = &prefixMappedErrorBody{ReadCloser: r.body, err: err}
		}
		resp.Body = r.body
		return streamPrefixResult{body: r.body}
	}
	for {
		select {
		case <-heartbeat:
			if observed.partial.Load() {
				continue
			}
			// Downstream-only heartbeat, never replay buffered upstream metadata and
			// never reset the semantic-output deadline or the upstream idle watchdog.
			if err := ping(); err != nil {
				heartbeat = nil
			}
		case <-idleDeadline:
			remaining := idle - time.Since(time.Unix(0, observed.lastRead.Load()))
			if remaining > 0 {
				idleTimer.Reset(remaining)
				continue
			}
			return handoffReadError(errStreamPrefixIdleTimeout)
		case r := <-result:
			if r.body != nil {
				resp.Body = r.body
			} else {
				_ = original.Close()
			}
			return r
		case <-ctx.Done():
			return handoffReadError(ctx.Err())
		case <-deadline:
			_ = original.Close()
			return streamPrefixResult{status: 502, reason: "OpenAI first output timeout", err: fmt.Errorf("first output timeout after %s", timeout)}
		}
	}
}
