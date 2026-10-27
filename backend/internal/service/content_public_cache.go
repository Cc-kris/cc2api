package service

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

// publicContentCacheTTL bounds how long a published snapshot may be served
// from the service-level cache. Mutations retire entries immediately through
// the snapshot version, so the TTL only bounds memory for unmutated content.
const publicContentCacheTTL = 60 * time.Second

// publicContentEntryCache is a bounded, snapshot-scoped cache used by the
// public content service (R9-10 / PERF-01). Each entry records the snapshot
// version it was built from: after InvalidatePublicContentCache the version
// moves forward and stale entries can never be served, even before they expire.
type publicContentEntryCache[T any] struct {
	ttl        time.Duration
	maxEntries int
	clone      func(T) T
	mu         sync.Mutex
	items      map[string]publicContentEntry[T]
	hits       atomic.Uint64
	misses     atomic.Uint64
}

type publicContentEntry[T any] struct {
	value     T
	version   uint64
	expiresAt time.Time
}

func newPublicContentEntryCache[T any](ttl time.Duration, maxEntries int, clone func(T) T) *publicContentEntryCache[T] {
	if ttl <= 0 {
		ttl = publicContentCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &publicContentEntryCache[T]{
		ttl:        ttl,
		maxEntries: maxEntries,
		clone:      clone,
		items:      make(map[string]publicContentEntry[T]),
	}
}

// Get returns a copy of the cached value when it still matches the current
// published snapshot and has not expired.
func (c *publicContentEntryCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil {
		return zero, false
	}
	version := PublicContentCacheVersion()
	c.mu.Lock()
	entry, ok := c.items[key]
	if ok && (entry.version != version || time.Now().After(entry.expiresAt)) {
		delete(c.items, key)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		c.misses.Add(1)
		return zero, false
	}
	c.hits.Add(1)
	if c.clone != nil {
		return c.clone(entry.value), true
	}
	return entry.value, true
}

// Put stores value as belonging to snapshot, which MUST be the version the
// caller captured before it started the repository read. A row queried while
// the snapshot was V and published over as V+1 stays tagged V, so readers at
// V+1 never serve it: the write-back can only claim the version it queried.
// Expired and not-current entries are swept first, and the whole map is
// dropped when it is still saturated (a bounded map, not an LRU). Callers must
// not mutate values they handed over afterwards.
func (c *publicContentEntryCache[T]) Put(key string, snapshot uint64, value T) {
	if c == nil {
		return
	}
	current := PublicContentCacheVersion()
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.maxEntries {
		for k, entry := range c.items {
			if now.After(entry.expiresAt) || entry.version != current {
				delete(c.items, k)
			}
		}
		if len(c.items) >= c.maxEntries {
			c.items = make(map[string]publicContentEntry[T], c.maxEntries)
		}
	}
	c.items[key] = publicContentEntry[T]{value: value, version: snapshot, expiresAt: now.Add(c.ttl)}
}

// Stats exposes hit/miss counters for diagnostics and perf evidence.
func (c *publicContentEntryCache[T]) Stats() (hits, misses uint64, size int) {
	if c == nil {
		return 0, 0, 0
	}
	c.mu.Lock()
	size = len(c.items)
	c.mu.Unlock()
	return c.hits.Load(), c.misses.Load(), size
}

// publicContentCacheKeyFor builds a version-free key for a logical view. The
// snapshot is deliberately NOT part of the key: entries carry their own version
// and are filtered on read, so a stale write-back lands on the same key instead
// of leaving unreachable entries behind for every published revision.
func publicContentCacheKeyFor(view, locale string, parts ...string) string {
	var b strings.Builder
	b.Grow(len(view) + len(locale) + 32)
	b.WriteString(strings.TrimSpace(view))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			b.WriteString("|")
			b.WriteString(part)
		}
	}
	if locale = strings.TrimSpace(locale); locale != "" {
		b.WriteString("?locale=")
		b.WriteString(locale)
	}
	return b.String()
}

func clonePublicNewsPage(page PublicNewsPage) PublicNewsPage {
	page.Items = append([]PublicNewsListItem(nil), page.Items...)
	for i := range page.Items {
		page.Items[i].Tags = append([]string(nil), page.Items[i].Tags...)
		page.Items[i].Alternates = append([]PublicAlternate(nil), page.Items[i].Alternates...)
	}
	return page
}

func clonePublicNews(item PublicNews) PublicNews {
	item.Tags = append([]string(nil), item.Tags...)
	item.Alternates = append([]PublicAlternate(nil), item.Alternates...)
	return item
}

func clonePublicArticle(item PublicArticle) PublicArticle {
	item.Tags = append([]string(nil), item.Tags...)
	item.Alternates = append([]PublicAlternate(nil), item.Alternates...)
	return item
}

func clonePublicArticlePage(page PublicArticlePage) PublicArticlePage {
	page.Items = append([]PublicArticleListItem(nil), page.Items...)
	for i := range page.Items {
		page.Items[i].Tags = append([]string(nil), page.Items[i].Tags...)
	}
	return page
}

func clonePublicArticleList(items []PublicArticleListItem) []PublicArticleListItem {
	out := append([]PublicArticleListItem(nil), items...)
	for i := range out {
		out[i].Tags = append([]string(nil), out[i].Tags...)
	}
	return out
}

// publicContentCaches groups the published-snapshot caches owned by a single
// PublicContentService instance. Per-instance ownership keeps isolated routers
// (tests, multi-tenant deploys) from sharing another instance's repository
// results while still agreeing on the global snapshot version for eviction.
type publicContentCaches struct {
	newsPage      *publicContentEntryCache[PublicNewsPage]
	newsDetail    *publicContentEntryCache[PublicNews]
	articlePage   *publicContentEntryCache[PublicArticlePage]
	articleDetail *publicContentEntryCache[PublicArticle]
	featured      *publicContentEntryCache[[]PublicArticleListItem]
}

func newPublicContentCaches() *publicContentCaches {
	return &publicContentCaches{
		newsPage:      newPublicContentEntryCache[PublicNewsPage](publicContentCacheTTL, 256, clonePublicNewsPage),
		newsDetail:    newPublicContentEntryCache[PublicNews](publicContentCacheTTL, 512, clonePublicNews),
		articlePage:   newPublicContentEntryCache[PublicArticlePage](publicContentCacheTTL, 128, clonePublicArticlePage),
		articleDetail: newPublicContentEntryCache[PublicArticle](publicContentCacheTTL, 512, clonePublicArticle),
		featured:      newPublicContentEntryCache[[]PublicArticleListItem](publicContentCacheTTL, 32, clonePublicArticleList),
	}
}
