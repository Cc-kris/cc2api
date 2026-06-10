package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize = 64 << 20
)

const RequestBodyTooLargeClientMessage = "上传的图片太大，请压缩上传的图片"

type RequestBodyReadErrorKind string

const (
	RequestBodyReadFailed             RequestBodyReadErrorKind = "read_failed"
	RequestBodyReadClientDisconnected RequestBodyReadErrorKind = "client_disconnected"
	RequestBodyReadIncompleteBody     RequestBodyReadErrorKind = "incomplete_body"
	RequestBodyReadTimeout            RequestBodyReadErrorKind = "read_timeout"
	RequestBodyDecodeFailed           RequestBodyReadErrorKind = "decode_failed"
	RequestBodyUnsupportedEncoding    RequestBodyReadErrorKind = "unsupported_encoding"
)

type RequestBodyReadError struct {
	Kind          RequestBodyReadErrorKind
	BytesRead     int64
	ContentLength int64
	Encoding      string
	Err           error
}

func (e *RequestBodyReadError) Error() string {
	if e == nil {
		return "request body read failed"
	}
	if e.Err == nil {
		return string(e.Kind)
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *RequestBodyReadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func RequestBodyReadErrorInfo(err error) (*RequestBodyReadError, bool) {
	var readErr *RequestBodyReadError
	if errors.As(err, &readErr) && readErr != nil {
		return readErr, true
	}
	return nil, false
}

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}

	buf := bytes.NewBuffer(make([]byte, 0, capHint))
	bytesRead, err := io.Copy(buf, req.Body)
	if err != nil {
		return nil, &RequestBodyReadError{
			Kind:          classifyRequestBodyReadError(err),
			BytesRead:     bytesRead,
			ContentLength: req.ContentLength,
			Encoding:      strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding"))),
			Err:           err,
		}
	}
	raw := buf.Bytes()

	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return raw, nil
	}

	decoded, err := decompressRequestBody(enc, raw)
	if err != nil {
		return nil, &RequestBodyReadError{
			Kind:          classifyRequestBodyDecodeError(enc, err),
			BytesRead:     bytesRead,
			ContentLength: req.ContentLength,
			Encoding:      enc,
			Err:           fmt.Errorf("decode Content-Encoding %q: %w", enc, err),
		}
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))

	return decoded, nil
}

func classifyRequestBodyReadError(err error) RequestBodyReadErrorKind {
	if err == nil {
		return RequestBodyReadFailed
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return RequestBodyReadClientDisconnected
	}
	if errors.Is(err, context.DeadlineExceeded) || isNetTimeout(err) {
		return RequestBodyReadTimeout
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return RequestBodyReadIncompleteBody
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "client disconnected"), strings.Contains(msg, "connection reset by peer"), strings.Contains(msg, "broken pipe"), strings.Contains(msg, "use of closed network connection"):
		return RequestBodyReadClientDisconnected
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return RequestBodyReadTimeout
	case strings.Contains(msg, "unexpected eof"), strings.Contains(msg, "early eof"):
		return RequestBodyReadIncompleteBody
	default:
		return RequestBodyReadFailed
	}
}

func classifyRequestBodyDecodeError(encoding string, err error) RequestBodyReadErrorKind {
	if strings.EqualFold(strings.TrimSpace(encoding), "") {
		return RequestBodyDecodeFailed
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unsupported content-encoding") {
		return RequestBodyUnsupportedEncoding
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unsupported") {
		return RequestBodyUnsupportedEncoding
	}
	return RequestBodyDecodeFailed
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return io.ReadAll(io.LimitReader(dec, maxDecompressedBodySize))
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gr.Close() }()
		return io.ReadAll(io.LimitReader(gr, maxDecompressedBodySize))
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return io.ReadAll(io.LimitReader(zr, maxDecompressedBodySize))
	default:
		return nil, errors.New("unsupported Content-Encoding")
	}
}
