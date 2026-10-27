#!/usr/bin/env python3
"""TASK-034 perf harness: real HTTP timings against an isolated sub2api instance."""
import argparse, json, os, statistics, subprocess, time
from concurrent.futures import ThreadPoolExecutor

import requests

S = requests.Session()
S.headers.update({"Accept": "text/html,application/json"})


def pct(vals, p):
    if not vals:
        return None
    vals = sorted(vals)
    k = (len(vals) - 1) * p / 100.0
    lo, hi = int(k), min(int(k) + 1, len(vals) - 1)
    return round(vals[lo] + (vals[hi] - vals[lo]) * (k - lo), 1)


def stats(name, samples_ms, extra=None):
    out = {
        "name": name,
        "n": len(samples_ms),
        "p50_ms": pct(samples_ms, 50),
        "p95_ms": pct(samples_ms, 95),
        "p99_ms": pct(samples_ms, 99),
        "min_ms": round(min(samples_ms), 1) if samples_ms else None,
        "max_ms": round(max(samples_ms), 1) if samples_ms else None,
        "mean_ms": round(statistics.fmean(samples_ms), 1) if samples_ms else None,
    }
    if extra:
        out.update(extra)
    return out


def timed(url, headers=None, expect=200):
    t0 = time.perf_counter()
    r = S.get(url, headers=headers or {}, timeout=60)
    dt = (time.perf_counter() - t0) * 1000
    if expect and r.status_code != expect:
        raise RuntimeError(f"{url} -> {r.status_code} {r.text[:200]}")
    return dt, len(r.content)


def rss_mb(pid):
    try:
        v = subprocess.check_output(["ps", "-o", "rss=", "-p", str(pid)], text=True).strip()
        return round(int(v) / 1024.0, 1) if v else None
    except Exception:
        return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:18080")
    ap.add_argument("--token-file", default="/tmp/t034/token.txt")
    ap.add_argument("--pid", type=int, default=0)
    ap.add_argument("--out", default="bench.json")
    ap.add_argument("--label", default="run")
    ap.add_argument("--reqs", type=int, default=100)
    ap.add_argument("--rounds", type=int, default=3)
    a = ap.parse_args()
    base = a.base.rstrip("/")
    raw = open(a.token_file).read()
    token = ""
    for ln in raw.splitlines():
        ln = ln.strip()
        if ln.startswith("JWT="):
            token = ln[4:]
        elif ln.count(".") == 2 and len(ln) > 40:
            token = ln
    assert token, "no JWT found"
    auth = {"Authorization": "Bearer " + token}

    frm, to = "2025-10-27", "2026-10-27"
    results = {"label": a.label, "base": base, "requests_per_round": a.reqs, "rounds": a.rounds,
               "started_at": time.strftime("%Y-%m-%dT%H:%M:%S"), "phases": [], "rss": {}}

    # sanity
    r = S.get(base + "/health", timeout=10)
    assert r.status_code == 200, r.status_code

    def phase(name, url_fn, headers=None, units=None):
        all_ms, sizes = [], []
        rounds = []
        for rd in range(a.rounds):
            ms = []
            for i in range(a.reqs):
                u = url_fn(i, rd)
                dt, size = timed(u, headers)
                ms.append(dt)
                sizes.append(size)
            rounds.append({"round": rd + 1, "p50_ms": pct(ms, 50), "p95_ms": pct(ms, 95),
                           "p99_ms": pct(ms, 99), "n": len(ms)})
            all_ms.extend(ms)
        st = stats(name, all_ms, {"rounds": rounds, "avg_body_bytes": int(statistics.fmean(sizes))})
        if units:
            st.update(units)
        results["phases"].append(st)
        print(f"[{name}] n={st['n']} p50={st['p50_ms']} p95={st['p95_ms']} p99={st['p99_ms']} bytes={st['avg_body_bytes']}", flush=True)
        return st

    # ---- PERF-01: SSR detail, cold (unique URLs) and hot (same URL) ----
    phase("ssr_detail_cold", lambda i, rd: f"{base}/news/zh/perf034-news-{10 * ((rd * a.reqs + i) % 700) + 3}")
    phase("ssr_detail_hot", lambda i, rd: f"{base}/news/zh/perf034-news-3")
    phase("ssr_detail_en_cold", lambda i, rd: f"{base}/news/en/perf034-news-{10 * ((rd * a.reqs + i) % 300) + 8}")

    # ---- PERF-01: SSR list, cold (unique page) and hot (same page) ----
    phase("ssr_list_cold", lambda i, rd: f"{base}/zh/news?page={1 + (rd * a.reqs + i) % 600}&page_size=10")
    phase("ssr_list_hot", lambda i, rd: f"{base}/zh/news?page=1&page_size=10")
    phase("ssr_articles_list_hot", lambda i, rd: f"{base}/zh/articles?page=1&page_size=10")

    # ---- R11-05: BI endpoints over 1 year of data ----
    phase("bi_overview_1y", lambda i, rd: f"{base}/api/v1/admin/bi/analytics/overview?from={frm}&to={to}", auth)
    phase("bi_distribution_1y", lambda i, rd: f"{base}/api/v1/admin/bi/analytics/distribution?from={frm}&to={to}&dim=locale", auth)
    phase("bi_distribution_source_1y", lambda i, rd: f"{base}/api/v1/admin/bi/analytics/distribution?from={frm}&to={to}&dim=source", auth)

    # ---- R2-03: admin list at 10,000 rows: page sizes, deep pages, filters ----
    r203 = [
        ("admin_list_10k_ps1", "page=1&page_size=1"),
        ("admin_list_10k_ps20", "page=1&page_size=20"),
        ("admin_list_10k_page500_ps20", "page=500&page_size=20"),
        ("admin_list_10k_search", "page=1&page_size=20&search=perf034"),
        ("admin_list_10k_status_locale", "page=1&page_size=20&status=published&locale=zh-CN"),
    ]
    for name, qs in r203:
        ms, lens = [], []
        for _ in range(20):
            dt, ln = timed(f"{base}/api/v1/admin/content/news?{qs}", auth)
            ms.append(dt); lens.append(ln)
        results["phases"].append(stats(name, ms, {"query": qs, "avg_body_bytes": int(statistics.fmean(lens))}))
        print(f"[{name}] n=20 p50={pct(ms,50)} p95={pct(ms,95)} bytes={int(statistics.fmean(lens))}", flush=True)

    # ---- PERF-02: concurrency + memory ----
    if a.pid:
        results["rss"]["before_concurrency_mb"] = rss_mb(a.pid)
    mix = [
        f"{base}/news/zh/perf034-news-13",
        f"{base}/zh/news?page=2&page_size=10",
        f"{base}/api/v1/admin/bi/analytics/overview?from={frm}&to={to}",
        f"{base}/api/v1/public/news?page=3&page_size=12",
    ]
    n_threads, per = 8, 25
    conc_ms = []

    def worker(k):
        out = []
        for j in range(per):
            u = mix[(k + j) % len(mix)]
            h = auth if "/admin/" in u else None
            dt, _ = timed(u, h)
            out.append(dt)
        return out

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=n_threads) as ex:
        for part in ex.map(worker, range(n_threads)):
            conc_ms.extend(part)
    wall = time.perf_counter() - t0
    st = stats("perf02_concurrency_mixed", conc_ms,
               {"threads": n_threads, "requests": len(conc_ms), "wall_s": round(wall, 2),
                "throughput_rps": round(len(conc_ms) / wall, 1)})
    results["phases"].append(st)
    print(f"[perf02_concurrency] n={st['n']} p95={st['p95_ms']} rps={st['throughput_rps']}", flush=True)
    if a.pid:
        time.sleep(1)
        results["rss"]["after_concurrency_mb"] = rss_mb(a.pid)

    results["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    json.dump(results, open(a.out, "w"), ensure_ascii=False, indent=2)
    print("WROTE", a.out)


if __name__ == "__main__":
    main()
