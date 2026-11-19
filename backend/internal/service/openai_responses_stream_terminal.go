package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAIResponsesTerminalWriter records terminal events at the service/output
// boundary. The handler still owns failures before a terminal event, but must
// not append a second generic response.failed after this stream already ended.
type responsesTerminalWriter struct {
	gin.ResponseWriter
	c *gin.Context
}

func restoreResponsesTerminalWriter(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	writer, ok := c.Writer.(*responsesTerminalWriter)
	if !ok || writer.ResponseWriter == nil {
		return
	}
	// Restore the exact pre-stream writer. The wrapper may itself wrap an ops
	// capture that its owner releases to a pool during request teardown.
	c.Writer = writer.ResponseWriter
}

func wrapResponsesTerminalWriter(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	if _, ok := c.Writer.(*responsesTerminalWriter); ok {
		return
	}
	c.Writer = &responsesTerminalWriter{ResponseWriter: c.Writer, c: c}
}

func (w *responsesTerminalWriter) Write(data []byte) (int, error) {
	if ResponsesTerminalEventWritten(w.c) && openAIResponsesStreamHasTerminal(string(data)) {
		// Claim the duplicate write so outer error handling does not retry it.
		return len(data), nil
	}
	n, err := w.ResponseWriter.Write(data)
	if n > 0 && openAIResponsesStreamHasTerminal(string(data[:n])) {
		w.c.Set(openAIResponsesTerminalWrittenKey, true)
	}
	return n, err
}

func (w *responsesTerminalWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// Delegate explicitly: Gin unwraps ResponseWriter chains and can otherwise reach
// a pooled wrapper whose underlying writer has already been released.
func (w *responsesTerminalWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *responsesTerminalWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

func (w *responsesTerminalWriter) WriteHeaderNow() {
	w.ResponseWriter.WriteHeaderNow()
}

func (w *responsesTerminalWriter) Status() int {
	return w.ResponseWriter.Status()
}

func (w *responsesTerminalWriter) Size() int {
	return w.ResponseWriter.Size()
}

func (w *responsesTerminalWriter) Written() bool {
	return w.ResponseWriter.Written()
}

func (w *responsesTerminalWriter) Flush() {
	w.ResponseWriter.Flush()
}

func (w *responsesTerminalWriter) Unwrap() gin.ResponseWriter {
	return w.ResponseWriter
}

func openAIResponsesStreamHasTerminal(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		switch strings.TrimSpace(gjson.Get(data, "type").String()) {
		case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			return true
		}
	}
	return false
}

// ResponsesTerminalEventWritten lets the outer Responses handler retain
// its fallback for other failures without duplicating a completed/failed turn.
const openAIResponsesTerminalWrittenKey = "openai_responses_terminal_event_written"

func ResponsesTerminalEventWritten(c *gin.Context) bool {
	return c != nil && c.GetBool(openAIResponsesTerminalWrittenKey)
}
