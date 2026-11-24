#!/usr/bin/env python3
"""Codex image_generation 桥接 —— 本地 E2E 驱动脚本。

前置：
  1) 起 mock 上游（e2e/codex-image-bridge/mock）：go run . -listen 127.0.0.1:19001 -log frames.jsonl
  2) 起被测 sub2api（分支 fix/codex-image-bridge-upstream），指向 mock 上游
  3) 重放 seed.sql 得到分组 11~15 与五个 API key（每个场景一个专用分组/账号）
  4) pip install requests websocket-client

用法：
  python3 run_e2e.py --frames /tmp/frames.jsonl
  python3 run_e2e.py --frames /tmp/frames.jsonl --only HTTP-08   # 单条复现

用例只读不改库：账号级开关 / strip 策略等场景都由 seed 静态描述。
（不要在用例里 UPDATE accounts 造场景——那走的是网关快照重建，0~30s 才生效，还会污染后续用例。）

判定口径（与官方 sub2api 语义一致）：
  - 注入 = 上游请求体带 tools[type=image_generation] + tool_choice=auto + instructions 含 <sub2api-codex-image-generation>
  - bridge 关闭 / 非 Codex 客户端 / responses-lite / compact / 分组禁生图 —— 均不得注入
  - WS ingress 下「普通原生生图轮」（桥接关闭 + 分组允许生图 + 请求体自带 image_generation 工具）
    刻意改走 HTTP 上游；纯文本轮与带 previous_response_id 的续写走 WS 上游
"""

import argparse
import json
import os
import subprocess
import sys
import time

import requests

MARKER = "<sub2api-codex-image-generation>"
LITE_HEADER = "X-OpenAI-Internal-Codex-Responses-Lite"
LITE_WS_META_KEY = "ws_request_header_x_openai_internal_codex_responses_lite"
CODEX_UA = "codex_cli_rs/0.166.0"
MODEL = "gpt-5.6-luna"
TINY_PNG_B64 = (
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
)

RESULTS = []


def record(name, ok, detail):
    RESULTS.append((name, ok, detail))
    print(("PASS  " if ok else "FAIL  ") + name + " — " + detail)


# ---------------------------------------------------------------- helpers

def clear_frames(path):
    with open(path, "w"):
        pass


def read_frames(path, retries=10, interval=0.2):
    for _ in range(retries):
        recs = []
        if os.path.exists(path):
            with open(path) as fh:
                recs = [json.loads(line) for line in fh if line.strip()]
        if recs:
            return recs
        time.sleep(interval)
    return []


def read_frame_for(path, needle, retries=15, interval=0.2):
    """按请求体内唯一 needle 精确取帧。

    mock 以 O_APPEND 常驻持有 fd，清空文件后迟到的旧帧仍可能落盘，
    所以不能只取"第一条非空帧"，必须按本次请求的唯一标记匹配。
    """
    for _ in range(retries):
        if os.path.exists(path):
            with open(path) as fh:
                for line in fh:
                    if not line.strip():
                        continue
                    rec = json.loads(line)
                    if needle in line:
                        return rec
        time.sleep(interval)
    return None


def uniq(tag):
    return "%s-%d-%d" % (tag, int(time.time() * 1000) % 1000000, os.getpid() % 1000)


def upstream_view(rec):
    body = rec.get("body") if isinstance(rec.get("body"), dict) else {}
    tools = body.get("tools")
    kinds = [t.get("type") for t in tools] if isinstance(tools, list) else tools
    return {
        "kind": rec.get("kind"),
        "path": rec.get("path"),
        "tools": kinds,
        "tool_choice": body.get("tool_choice"),
        "marker": MARKER in (body.get("instructions") or ""),
        "prev": body.get("previous_response_id"),
    }


def text_input(text="hello"):
    return [{"type": "message", "role": "user", "content": [{"type": "input_text", "text": text}]}]


def image_input(text="what is this"):
    return [{
        "type": "message",
        "role": "user",
        "content": [
            {"type": "input_text", "text": text},
            {"type": "input_image", "image_url": "data:image/png;base64," + TINY_PNG_B64},
        ],
    }]


def post_responses(base, key, payload, headers=None, path="/v1/responses", ua=CODEX_UA):
    h = {
        "Authorization": "Bearer " + key,
        "User-Agent": ua,
        "originator": "codex_cli_rs",
        "Content-Type": "application/json",
    }
    if headers:
        h.update(headers)
    return requests.post(base.rstrip("/") + path, headers=h, data=json.dumps(payload), timeout=60)


def ws_send(ws_base, key, payload, headers=None, ua=CODEX_UA, originator="codex_cli_rs", max_events=8):
    import websocket  # websocket-client

    h = ["Authorization: Bearer " + key, "User-Agent: " + ua, "originator: " + originator]
    for k, v in (headers or {}).items():
        h.append(f"{k}: {v}")
    ws = websocket.create_connection(ws_base.rstrip("/") + "/v1/responses", header=h, timeout=30)
    try:
        ws.send(json.dumps(payload))
        events = []
        for _ in range(max_events):
            raw = ws.recv()
            events.append(json.loads(raw))
            if events[-1].get("type") in ("response.completed", "error"):
                break
        return events
    finally:
        ws.close()


def image_items(events):
    items = []
    for ev in events:
        kind = ev.get("type")
        if kind == "response.output_item.done" and isinstance(ev.get("item"), dict):
            items.append(ev["item"])
        elif kind in ("response.completed", "response.done"):
            resp = ev.get("response") or {}
            items.extend(resp.get("output") or [])
    return [i for i in items if i.get("type") == "image_generation_call" and i.get("result")]


# ---------------------------------------------------------------- HTTP cases

def case_http_text_injects(args):
    tag = uniq("http01")
    clear_frames(args.frames)
    r = post_responses(args.base, args.key_bridge_on, {"model": MODEL, "stream": False, "input": text_input(tag)})
    rec = read_frame_for(args.frames, tag)
    if r.status_code != 200 or rec is None:
        return record("HTTP-01 纯文本(codex)注入", False, f"status={r.status_code} frames=0")
    v = upstream_view(rec)
    ok = v["kind"] == "http" and v["tools"] == ["image_generation"] and v["tool_choice"] == "auto" and v["marker"]
    record("HTTP-01 纯文本(codex)注入", ok, json.dumps(v, ensure_ascii=False))


def case_http_bridge_off(args):
    tag = uniq("http02")
    clear_frames(args.frames)
    r = post_responses(args.base, args.key_bridge_off, {"model": MODEL, "stream": False, "input": text_input(tag)})
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("HTTP-02 渠道关闭桥接不注入", False, f"status={r.status_code} 无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] is None and v["tool_choice"] is None and not v["marker"]
    record("HTTP-02 渠道关闭桥接不注入", ok, json.dumps(v, ensure_ascii=False))


def case_http_compact_exempt(args):
    tag = uniq("http03")
    clear_frames(args.frames)
    r = post_responses(args.base, args.key_bridge_on, {"model": MODEL, "stream": False, "input": text_input(tag)},
                       path="/v1/responses/compact")
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("HTTP-03 /responses/compact 不注入", False, f"status={r.status_code} 无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] is None and not v["marker"]
    record("HTTP-03 /responses/compact 不注入", ok, json.dumps(v, ensure_ascii=False))


def case_http_lite_header(args):
    tag = uniq("http04")
    clear_frames(args.frames)
    r = post_responses(args.base, args.key_bridge_on, {"model": MODEL, "stream": False, "input": text_input(tag)},
                       headers={LITE_HEADER: "true"})
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("HTTP-04 responses-lite 不注入", False, f"status={r.status_code} 无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] is None and not v["marker"]
    record("HTTP-04 responses-lite 不注入", ok, json.dumps(v, ensure_ascii=False))


def case_http_non_codex_ua(args):
    tag = uniq("http05")
    clear_frames(args.frames)
    h = {"Authorization": "Bearer " + args.key_bridge_on, "User-Agent": "curl/8.7.1",
         "Content-Type": "application/json"}
    r = requests.post(args.base.rstrip("/") + "/v1/responses", headers=h,
                      data=json.dumps({"model": MODEL, "stream": False, "input": text_input(tag)}), timeout=60)
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("HTTP-05 非 Codex 客户端不注入", False, f"status={r.status_code} 无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] is None and not v["marker"]
    record("HTTP-05 非 Codex 客户端不注入", ok, json.dumps(v, ensure_ascii=False))


def case_http_group_denied(args):
    """分组 allow_image_generation=false：显式选图 / 原生 image_generation 工具 → 403 且不上送。"""
    payloads = {
        "tool_choice 显式选图": {
            "model": MODEL, "stream": False,
            "tools": [{"type": "image_generation", "model": "gpt-image-1"}],
            "tool_choice": {"type": "function", "function": {"name": "image_generation"}},
            "input": text_input("draw a cat"),
        },
        "原生 image_generation 工具": {
            "model": MODEL, "stream": False,
            "tools": [{"type": "image_generation", "model": "gpt-image-1"}],
            "input": text_input("draw a cat"),
        },
    }
    for label, payload in payloads.items():
        payload["input"] = text_input(uniq("http06"))
        clear_frames(args.frames)
        r = post_responses(args.base, args.key_img_denied, payload)
        rec = read_frame_for(args.frames, payload["input"][0]["content"][0]["text"], retries=3)
        ok = r.status_code == 403 and rec is None
        record(f"HTTP-06 分组禁生图·{label}", ok, f"status={r.status_code} 上游帧={0 if rec is None else 1} body={r.text[:120]}")


def case_http_end_to_end_image(args):
    tag = uniq("http07")
    clear_frames(args.frames)
    payload = {"model": MODEL, "stream": False, "input": text_input("DRAW a red square " + tag)}
    r = post_responses(args.base, args.key_bridge_on, payload)
    rec = read_frame_for(args.frames, tag)
    items = []
    if r.status_code == 200:
        try:
            body = r.json()
            items = [i for i in (body.get("output") or []) if i.get("type") == "image_generation_call"]
        except Exception:
            pass
    ok = r.status_code == 200 and bool(items) and bool(items[0].get("result")) and rec is not None
    record("HTTP-07 端到端出图", ok,
           f"status={r.status_code} image_items={len(items)} 有result={bool(items and items[0].get('result'))}")


# ---------------------------------------------------------------- WS cases

def case_ws_text_injects(args):
    tag = uniq("ws01")
    clear_frames(args.frames)
    events = ws_send(args.ws_base, args.key_bridge_on, {"model": MODEL, "input": text_input(tag)})
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("WS-01 纯文本轮走 WS 上游并注入", False, f"events={[e.get('type') for e in events]} 无上游帧")
    v = upstream_view(rec)
    ok = v["kind"] == "ws" and v["tools"] == ["image_generation"] and v["tool_choice"] == "auto" and v["marker"]
    record("WS-01 纯文本轮走 WS 上游并注入", ok, json.dumps(v, ensure_ascii=False))


def case_ws_image_turn_uses_http(args):
    """桥接关闭 + 分组允许生图 + 原生 image_generation 工具 → WS 入站刻意改走 HTTP 上游。

    ordinaryNativeImage 成立条件（backend/handler openai_gateway_handler.go:1720）：
      !route.Enabled && GroupAllowsImageGeneration && 模型非生图模型 && IsImageGenerationIntent(payload)
    因此这一轮必须用 bridge-off 的 key，且请求体自带 image_generation 工具。
    """
    tag = uniq("ws02")
    clear_frames(args.frames)
    payload = {"model": MODEL, "input": text_input(tag),
               "tools": [{"type": "image_generation", "model": "gpt-image-1"}]}
    events = ws_send(args.ws_base, args.key_bridge_off, payload)
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("WS-02 原生生图轮改走 HTTP 上游", False, f"events={[e.get('type') for e in events]} 无上游帧")
    v = upstream_view(rec)
    ok = v["kind"] == "http" and v["tools"] == ["image_generation"]
    record("WS-02 原生生图轮改走 HTTP 上游", ok, json.dumps(v, ensure_ascii=False))


def case_ws_previous_response(args):
    tag1, tag2 = uniq("ws03a"), uniq("ws03b")
    clear_frames(args.frames)
    events = ws_send(args.ws_base, args.key_bridge_on, {"model": MODEL, "input": text_input(tag1)})
    first = read_frame_for(args.frames, tag1)
    if first is None:
        return record("WS-03 带 previous_response_id 续写走 WS 并注入", False, "首轮无上游帧")
    # 取网关回给客户端的 response id，作为续写 id
    resp_id = None
    for ev in events:
        rid = (ev.get("response") or {}).get("id") if isinstance(ev.get("response"), dict) else None
        if rid:
            resp_id = rid
            break
    if not resp_id:
        return record("WS-03 带 previous_response_id 续写走 WS 并注入", False,
                      f"首轮无 response id events={[e.get('type') for e in events]}")
    clear_frames(args.frames)
    ws_send(args.ws_base, args.key_bridge_on,
            {"model": MODEL, "input": text_input(tag2), "previous_response_id": resp_id})
    rec = read_frame_for(args.frames, tag2)
    if rec is None:
        return record("WS-03 带 previous_response_id 续写走 WS 并注入", False, "续写无上游帧")
    v = upstream_view(rec)
    # previous_response_id 会被网关重映射成上游 id，因此只断言"仍带 previous_response_id 且走 WS + 注入"
    ok = v["kind"] == "ws" and v["tools"] == ["image_generation"] and bool(v["prev"])
    record("WS-03 带 previous_response_id 续写走 WS 并注入", ok,
           json.dumps(dict(v, client_prev=resp_id), ensure_ascii=False))


def case_ws_lite(args):
    tag = uniq("ws04")
    clear_frames(args.frames)
    payload = {"model": MODEL, "input": text_input(tag),
               "client_metadata": {LITE_WS_META_KEY: "true"}}
    ws_send(args.ws_base, args.key_bridge_on, payload)
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("WS-04 responses-lite(WS 元数据)不注入", False, "无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] is None and not v["marker"]
    record("WS-04 responses-lite(WS 元数据)不注入", ok, json.dumps(v, ensure_ascii=False))


def case_ws_non_codex(args):
    tag = uniq("ws05")
    clear_frames(args.frames)
    ws_send(args.ws_base, args.key_bridge_on, {"model": MODEL, "input": text_input(tag)},
            ua="python-websocket/1.0", originator="")
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("WS-05 非 Codex 客户端不注入", False, "无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] is None and not v["marker"]
    record("WS-05 非 Codex 客户端不注入", ok, json.dumps(v, ensure_ascii=False))


# ---------------------------------------------------------------- 账号级策略用例（只读，场景见 seed）

def case_account_level_override(args):
    """渠道关闭桥接，但账号 extra 显式开启 → 注入（账号级优先于渠道）。

    场景由 seed 静态描述（group 14 / 账号 102 / 渠道 24 关闭），用例只读不改库：
    直接 UPDATE accounts 走的是快照重建，生效时机 0~30s 抖动，还会污染后续用例。
    """
    tag = uniq("http08")
    clear_frames(args.frames)
    r = post_responses(args.base, args.key_account_bridge, {"model": MODEL, "stream": False, "input": text_input(tag)})
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("HTTP-08 账号级覆盖优先于渠道", False, f"status={r.status_code} 无上游帧")
    v = upstream_view(rec)
    ok = v["tools"] == ["image_generation"] and v["tool_choice"] == "auto" and v["marker"]
    record("HTTP-08 账号级覆盖优先于渠道", ok, json.dumps(v, ensure_ascii=False))


def case_strip_policy(args):
    """账号策略 explicit_tool_policy=strip：客户端自带的 image_generation 声明被剔除。

    场景由 seed 静态描述（group 15 / 账号 103 strip / 渠道 25 开桥接），只读不改库。
    """
    tag = uniq("http09")
    clear_frames(args.frames)
    payload = {
        "model": MODEL, "stream": False,
        "tools": [{"type": "image_generation", "model": "gpt-image-1"}],
        "input": text_input(tag),
    }
    r = post_responses(args.base, args.key_strip_policy, payload)
    rec = read_frame_for(args.frames, tag)
    if rec is None:
        return record("HTTP-09 strip 策略剥工具声明", False, f"status={r.status_code} 无上游帧")
    v = upstream_view(rec)
    # strip：客户端声明的 image_generation 被剔除，桥接也因 strip 策略不注入
    ok = v["tools"] is None and not v["marker"]
    record("HTTP-09 strip 策略剥工具声明", ok, json.dumps(v, ensure_ascii=False))


# ---------------------------------------------------------------- main

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:18082")
    ap.add_argument("--ws-base", default="ws://127.0.0.1:18082")
    ap.add_argument("--frames", default="frames.jsonl", help="mock 上游的帧记录文件")
    ap.add_argument("--key-bridge-on", default="sk-e2e-bridge-on-00000000000000000000000000000000")
    ap.add_argument("--key-bridge-off", default="sk-e2e-bridge-off-0000000000000000000000000000000")
    ap.add_argument("--key-img-denied", default="sk-e2e-img-denied-0000000000000000000000000000000")
    ap.add_argument("--key-account-bridge", default="sk-e2e-account-bridge-000000000000000000000000000")
    ap.add_argument("--key-strip-policy", default="sk-e2e-strip-policy-0000000000000000000000000000")
    ap.add_argument("--only", default="", help="只跑名字含该子串的用例（调试用，如 http08 / HTTP-08）")
    args = ap.parse_args()

    if not os.path.exists(args.frames):
        sys.exit(f"找不到帧记录文件 {args.frames}：先起 mock 上游并让它写这个文件")

    cases = [
        ("HTTP-01", case_http_text_injects),
        ("HTTP-02", case_http_bridge_off),
        ("HTTP-03", case_http_compact_exempt),
        ("HTTP-04", case_http_lite_header),
        ("HTTP-05", case_http_non_codex_ua),
        ("HTTP-06", case_http_group_denied),
        ("HTTP-07", case_http_end_to_end_image),
        ("HTTP-08", case_account_level_override),
        ("HTTP-09", case_strip_policy),
        ("WS-01", case_ws_text_injects),
        ("WS-02", case_ws_image_turn_uses_http),
        ("WS-03", case_ws_previous_response),
        ("WS-04", case_ws_lite),
        ("WS-05", case_ws_non_codex),
    ]
    if args.only:
        needle = args.only.lower()
        cases = [(n, f) for n, f in cases if needle in n.lower()]
        if not cases:
            sys.exit(f"--only {args.only} 没有匹配的用例")
    for name, fn in cases:
        fn(args)

    failed = [n for n, ok, _ in RESULTS if not ok]
    print("\n== 汇总：%d 通过 / %d 失败 ==" % (len(RESULTS) - len(failed), len(failed)))
    if failed:
        for n in failed:
            print("  FAIL " + n)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
