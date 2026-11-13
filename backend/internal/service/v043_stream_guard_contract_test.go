//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestV043DetachedUpstreamPreCanceledParentIsSynchronous(t *testing.T) {
	for range 500 {
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		ctx, release := detachUpstreamContextWithTimeout(parent, time.Minute)
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		release()
		ReleaseDetachedUpstreamContext(ctx)
	}
}

func TestV043PrefixHeartbeatDoesNotReleaseFailedMetadata(t *testing.T) {
	for _, anthropic := range []bool{false, true} {
		t.Run(map[bool]string{true: "anthropic", false: "responses"}[anthropic], func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer writer.Close()
				if anthropic {
					_, _ = io.WriteString(writer, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"failed-secret\"}}\n\n")
				} else {
					_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-secret\"}}\n\n")
				}
				timer := time.NewTimer(80 * time.Millisecond)
				defer timer.Stop()
				<-timer.C
				_, _ = io.WriteString(writer, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"code\":\"server_is_overloaded\"}}\n\n")
			}()
			response := &http.Response{Body: reader}
			heartbeats := 0
			result := guardStreamFirstOutputWithKeepalive(context.Background(), response, time.Second, anthropic, 0, 10*time.Millisecond, func() error { heartbeats++; return nil })
			<-done
			require.Equal(t, 529, result.status)
			require.Positive(t, heartbeats)
			require.Nil(t, result.body, "failed metadata must never be replayed")
			require.NotContains(t, string(result.payload), "failed-secret")
		})
	}
}

func TestV043PrefixMetadataAndHeartbeatDoNotResetAbsoluteDeadline(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := io.WriteString(writer, "data: {\"type\":\"response.in_progress\"}\n\n"); err != nil {
				return
			}
		}
	}()
	response := &http.Response{Body: reader}
	heartbeats := 0
	started := time.Now()
	result := guardStreamFirstOutputWithKeepalive(context.Background(), response, 100*time.Millisecond, false, 80*time.Millisecond, 5*time.Millisecond, func() error { heartbeats++; return nil })
	<-done
	require.Equal(t, 502, result.status)
	require.True(t, strings.Contains(result.reason, "first output timeout"), result.reason)
	require.Positive(t, heartbeats)
	require.Less(t, time.Since(started), time.Second)
}
