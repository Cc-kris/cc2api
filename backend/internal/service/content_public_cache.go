package service

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	publicContentCacheInvalidatorsMu sync.RWMutex
	publicContentCacheInvalidators   []func()
	// publicContentCacheVersion is the published-content snapshot version. It
	// advances on every successful mutation so renderers/caches can tag entries
	// with the snapshot they were built from (R9-10).
	publicContentCacheVersion atomic.Uint64
)

// RegisterPublicContentCacheInvalidator connects content mutations to the
// renderer's public HTML cache without coupling the service package to HTTP.
// Listeners run in registration order; registering the same listener twice is
// the caller's responsibility.
func RegisterPublicContentCacheInvalidator(fn func()) {
	if fn == nil {
		return
	}
	publicContentCacheInvalidatorsMu.Lock()
	publicContentCacheInvalidators = append(publicContentCacheInvalidators, fn)
	publicContentCacheInvalidatorsMu.Unlock()
}

// InvalidatePublicContentCache invalidates published HTML/list/sitemap output
// after a successful content mutation. The version is bumped before the
// listeners run so output produced by an in-flight request can never be stored
// under the new snapshot. It is intentionally safe when no renderer is
// installed (CLI/tests).
func InvalidatePublicContentCache() {
	publicContentCacheVersion.Add(1)
	publicContentCacheInvalidatorsMu.RLock()
	listeners := append([]func(){}, publicContentCacheInvalidators...)
	publicContentCacheInvalidatorsMu.RUnlock()
	for _, fn := range listeners {
		fn()
	}
}

// PublicContentCacheVersion reports the current published-content snapshot.
// It only moves forward, so it is safe to embed in cache keys or expose for
// cache diagnostics (verifying a mutation really retired the previous HTML).
func PublicContentCacheVersion() uint64 {
	return publicContentCacheVersion.Load()
}

// PublicContentCacheKey builds a cache key scoped to content path, locale and
// the current snapshot version, so a published/unpublished translation can
// never be served from a key baked before the mutation (R9-10).
func PublicContentCacheKey(path, locale string) string {
	var b strings.Builder
	b.Grow(len(path) + len(locale) + 24)
	b.WriteString(strings.TrimSpace(path))
	if locale = strings.TrimSpace(locale); locale != "" {
		b.WriteString("?locale=")
		b.WriteString(locale)
		b.WriteString("&")
	} else {
		b.WriteString("?")
	}
	b.WriteString("v=")
	b.WriteString(strconv.FormatUint(PublicContentCacheVersion(), 10))
	return b.String()
}
