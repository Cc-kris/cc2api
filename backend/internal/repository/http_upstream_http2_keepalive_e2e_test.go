package repository

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

// 端到端补充（V02-AC40）：98027cdde 的测试只看字段；这里真连一次 TLS 服务端，
// 确认 openai_h2 协商出 h2（直连和 HTTP CONNECT 代理两种）、h1_fallback 不受影响、
// 连接空闲后客户端真的发出 PING。

func newH2TestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func trustTestServer(tr *http.Transport, srv *httptest.Server) {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	}
	tr.TLSClientConfig.RootCAs = pool
}

// startConnectProxy 起一个最小 HTTP CONNECT 代理，返回代理 URL 和 CONNECT 次数。
func startConnectProxy(t *testing.T) (*url.URL, *int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var connects int32
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			go func(client net.Conn) {
				defer func() { _ = client.Close() }()
				req, err := http.ReadRequest(bufio.NewReader(client))
				if err != nil || req.Method != http.MethodConnect {
					return
				}
				atomic.AddInt32(&connects, 1)
				target, err := net.Dial("tcp", req.Host)
				if err != nil {
					_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
					return
				}
				defer func() { _ = target.Close() }()
				_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
				go func() { _, _ = io.Copy(target, client) }()
				_, _ = io.Copy(client, target)
			}(client)
		}
	}()
	proxyURL, err := url.Parse("http://" + ln.Addr().String())
	require.NoError(t, err)
	return proxyURL, &connects
}

func getProto(t *testing.T, tr *http.Transport, target string) (int, string) {
	t.Helper()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get(target)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.ProtoMajor, string(body)
}

func TestBuildUpstreamTransport_OpenAIH2_NegotiatesH2(t *testing.T) {
	srv := newH2TestServer(t)

	t.Run("direct", func(t *testing.T) {
		tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, upstreamProtocolModeOpenAIH2)
		require.NoError(t, err)
		trustTestServer(tr, srv)
		defer tr.CloseIdleConnections()

		major, serverProto := getProto(t, tr, srv.URL)
		require.Equal(t, 2, major)
		require.Equal(t, "HTTP/2.0", serverProto)
	})

	t.Run("http_connect_proxy", func(t *testing.T) {
		proxyURL, connects := startConnectProxy(t)
		tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), proxyURL, upstreamProtocolModeOpenAIH2)
		require.NoError(t, err)
		trustTestServer(tr, srv)
		defer tr.CloseIdleConnections()

		major, serverProto := getProto(t, tr, srv.URL)
		require.Equal(t, 2, major)
		require.Equal(t, "HTTP/2.0", serverProto)
		require.EqualValues(t, 1, atomic.LoadInt32(connects), "请求必须经过 HTTP 代理")
	})
}

// openai_h1_fallback（本地 HTTP/2 代理回退）不经过 http2.ConfigureTransports，仍走 HTTP/1.1。
func TestBuildUpstreamTransport_OpenAIH1Fallback_UnaffectedByKeepAlive(t *testing.T) {
	srv := newH2TestServer(t)
	tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, upstreamProtocolModeOpenAIH1Fallback)
	require.NoError(t, err)
	require.Nil(t, tr.Protocols)
	require.Empty(t, tr.TLSNextProto)
	trustTestServer(tr, srv)
	defer tr.CloseIdleConnections()

	major, _ := getProto(t, tr, srv.URL)
	require.Equal(t, 1, major)
}

// frameSniffConn 解析服务端读到的 HTTP/2 明文帧（TLS 之上），记录收到的 PING。
type frameSniffConn struct {
	net.Conn
	mu       sync.Mutex
	buf      []byte
	preface  bool
	pingSeen chan struct{}
	once     sync.Once
}

func (c *frameSniffConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.observe(p[:n])
	}
	return n, err
}

func (c *frameSniffConn) observe(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, data...)
	if !c.preface {
		if len(c.buf) < len(http2.ClientPreface) {
			return
		}
		c.buf = c.buf[len(http2.ClientPreface):]
		c.preface = true
	}
	for len(c.buf) >= 9 {
		length := int(c.buf[0])<<16 | int(c.buf[1])<<8 | int(c.buf[2])
		if len(c.buf) < 9+length {
			return
		}
		frameType := http2.FrameType(c.buf[3])
		flags := http2.Flags(c.buf[4])
		if frameType == http2.FramePing && !flags.Has(http2.FlagPingAck) {
			c.once.Do(func() { close(c.pingSeen) })
		}
		c.buf = c.buf[9+length:]
	}
}

// 连接空闲 ReadIdleTimeout 后客户端主动发 PING：确认 enableOpenAIHTTP2KeepAlive
// 返回的 *http2.Transport 上设的超时在 Go 1.26（旧实现）和 Go 1.27（RegisterProtocol）
// 两种实现下都真正生效。测试里把 ReadIdleTimeout 调成 200ms。
func TestEnableOpenAIHTTP2KeepAlive_SendsPingOnIdleConnection(t *testing.T) {
	cert := httptest.NewUnstartedServer(http.NotFoundHandler())
	cert.EnableHTTP2 = true
	cert.StartTLS()
	certificate := cert.Certificate()
	tlsCert := cert.TLS.Certificates[0]
	cert.Close()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{http2.NextProtoTLS},
	})
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	sniff := make(chan *frameSniffConn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return
		}
		wrapped := &frameSniffConn{Conn: tlsConn, pingSeen: make(chan struct{})}
		sniff <- wrapped
		(&http2.Server{}).ServeConn(wrapped, &http2.ServeConnOpts{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "ok")
			}),
		})
	}()

	tr := &http.Transport{ForceAttemptHTTP2: true}
	h2, err := enableOpenAIHTTP2KeepAlive(tr)
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, h2.ReadIdleTimeout)
	require.Equal(t, 15*time.Second, h2.PingTimeout)
	h2.ReadIdleTimeout = 200 * time.Millisecond
	h2.PingTimeout = time.Second
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	tr.TLSClientConfig.RootCAs = pool
	tr.TLSClientConfig.ServerName = "example.com"
	defer tr.CloseIdleConnections()

	major, body := getProto(t, tr, "https://"+ln.Addr().String())
	require.Equal(t, 2, major)
	require.Equal(t, "ok", body)

	var conn *frameSniffConn
	select {
	case conn = <-sniff:
	case <-time.After(5 * time.Second):
		t.Fatal("server never accepted the connection")
	}
	select {
	case <-conn.pingSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("client did not send an HTTP/2 PING on the idle connection")
	}
}
