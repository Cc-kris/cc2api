// Codex image bridge E2E — 上游 mock（HTTP + WebSocket 同端口）
//
// 用途：作为 sub2api 的 openai apikey 上游，记录网关真正上送的每一帧，
// 并在 payload 含 "DRAW" 时返回 image_generation_call，用于验证
// hosted image_generation 工具注入的端到端链路。
//
// 用法：go run . -listen 127.0.0.1:19001 -log frames.jsonl
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	mu   sync.Mutex
	out  *os.File
	seq  int
	upgr = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
)

// 1x1 透明 PNG，避免把二进制大对象写进仓库。
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func record(kind, path string, header http.Header, raw []byte) {
	mu.Lock()
	defer mu.Unlock()
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		body = string(raw)
	}
	rec := map[string]any{
		"kind": kind,
		"path": path,
		"header": map[string]string{
			"user-agent": header.Get("User-Agent"),
			"originator": header.Get("originator"),
		},
		"body": body,
	}
	b, _ := json.Marshal(rec)
	fmt.Fprintln(out, string(b))
	_ = out.Sync()
}

func wantImage(raw []byte) bool { return bytes.Contains(raw, []byte("DRAW")) }

func imageFrames(id string) []map[string]any {
	item := map[string]any{
		"id": "ig_" + id, "type": "image_generation_call", "status": "completed",
		"result": tinyPNG, "output_format": "png",
	}
	return []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": id, "model": "gpt-5.6-luna"}},
		{"type": "response.output_item.done", "item": item},
		{"type": "response.completed", "response": map[string]any{
			"id": id, "model": "gpt-5.6-luna", "output": []any{item},
			"usage": map[string]any{"input_tokens": 5, "output_tokens": 1},
		}},
	}
}

func textFrames(id string) []map[string]any {
	return []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": id, "model": "gpt-5.6-luna"}},
		{"type": "response.completed", "response": map[string]any{
			"id": id, "model": "gpt-5.6-luna",
			"usage": map[string]any{"input_tokens": 5, "output_tokens": 1},
		}},
	}
}

func framesFor(id string, raw []byte) []map[string]any {
	if wantImage(raw) {
		return imageFrames(id)
	}
	return textFrames(id)
}

func writeSSE(w http.ResponseWriter, id string, raw []byte) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	for _, f := range framesFor(id, raw) {
		b, _ := json.Marshal(f)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", f["type"], b)
		if fl != nil {
			fl.Flush()
		}
	}
}

func nextID(prefix string) string {
	mu.Lock()
	defer mu.Unlock()
	seq++
	return fmt.Sprintf("%s_%d", prefix, seq)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:19001", "监听地址")
	logPath := flag.String("log", "frames.jsonl", "帧记录文件（JSONL，每次请求一行）")
	flag.Parse()

	var err error
	out, err = os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			conn, err := upgr.Upgrade(w, r, nil)
			if err != nil {
				log.Println("upgrade:", err)
				return
			}
			defer conn.Close()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				record("ws", r.URL.Path, r.Header, msg)
				for _, f := range framesFor(nextID("resp_ws"), msg) {
					if err := conn.WriteJSON(f); err != nil {
						return
					}
				}
			}
		}
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			record("http", r.URL.Path, r.Header, raw)
			writeSSE(w, nextID("resp_http"), raw)
			return
		}
		_, _ = w.Write([]byte("{}"))
	})
	log.Printf("mock listening on %s, log=%s", *listen, *logPath)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
