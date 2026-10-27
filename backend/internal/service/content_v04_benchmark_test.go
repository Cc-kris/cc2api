package service

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingPublicContentRepo wraps the shared stub and counts repository hits so
// tests can prove the service-level snapshot cache absorbs repeat reads.
type countingPublicContentRepo struct {
	*publicContentRepoStub
	listNewsCalls atomic.Int64
	getNewsCalls  atomic.Int64
	listArtCalls  atomic.Int64
	getArtCalls   atomic.Int64
}

func (r *countingPublicContentRepo) ListPublishedNews(ctx context.Context, locale string, limit, offset int) (PublicNewsPage, error) {
	r.listNewsCalls.Add(1)
	return r.publicContentRepoStub.ListPublishedNews(ctx, locale, limit, offset)
}

func (r *countingPublicContentRepo) GetPublishedNews(ctx context.Context, locale, slug string) (*PublicNews, error) {
	r.getNewsCalls.Add(1)
	return r.publicContentRepoStub.GetPublishedNews(ctx, locale, slug)
}

func (r *countingPublicContentRepo) ListPublishedArticles(ctx context.Context, locale string, limit, offset int) (PublicArticlePage, error) {
	r.listArtCalls.Add(1)
	return r.publicContentRepoStub.ListPublishedArticles(ctx, locale, limit, offset)
}

func (r *countingPublicContentRepo) GetPublishedArticle(ctx context.Context, locale, slug string) (*PublicArticle, error) {
	r.getArtCalls.Add(1)
	return r.publicContentRepoStub.GetPublishedArticle(ctx, locale, slug)
}

func newPerfRepo() *countingPublicContentRepo {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	news := make([]PublicNewsListItem, 0, 20)
	for i := 0; i < 20; i++ {
		news = append(news, PublicNewsListItem{
			Locale: "zh-CN", Slug: "perf-news-" + strconv.Itoa(i), Title: "Perf news " + strconv.Itoa(i),
			Summary: "Summary", SourceName: "Source", PublishedAt: publishedAt, Status: "published",
		})
	}
	articles := []PublicArticleListItem{{
		Locale: "zh-CN", Slug: "perf-article", Title: "Perf article", Excerpt: "Excerpt",
		PublishedAt: publishedAt, Status: "published",
	}}
	return &countingPublicContentRepo{publicContentRepoStub: &publicContentRepoStub{
		news:     news,
		articles: articles,
		detailNews: []PublicNews{{
			Locale: "zh-CN", Slug: "perf-news-0", Title: "Perf news 0", Summary: "Summary",
			SourceName: "Source", BodyHTML: "<p>body</p>", PublishedAt: publishedAt, Status: "published",
		}},
		detailArticles: []PublicArticle{{
			Locale: "zh-CN", Slug: "perf-article", Title: "Perf article", Excerpt: "Excerpt",
			BodyHTML: "<p>body</p>", PublishedAt: publishedAt, Status: "published",
		}},
	}}
}

// BenchmarkPublicContentNewsListWarm measures the cached steady-state cost of a
// public news list read, which is the hot path of the SSR news index.
func BenchmarkPublicContentNewsListWarm(b *testing.B) {
	repo := newPerfRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()
	if _, err := svc.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		b.Fatalf("warmup: %v", err)
	}
	repo.listNewsCalls.Store(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.ListNews(ctx, "zh-CN", 1, 20); err != nil {
			b.Fatalf("ListNews: %v", err)
		}
	}
	b.StopTimer()
	if calls := repo.listNewsCalls.Load(); calls != 0 {
		b.Fatalf("expected 0 repository calls for warm reads, got %d", calls)
	}
}

// BenchmarkPublicContentNewsDetailWarm measures the cached steady-state cost of
// a public news detail render (SSR article page).
func BenchmarkPublicContentNewsDetailWarm(b *testing.B) {
	repo := newPerfRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()
	if _, err := svc.GetNews(ctx, "zh-CN", "perf-news-0"); err != nil {
		b.Fatalf("warmup: %v", err)
	}
	repo.getNewsCalls.Store(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetNews(ctx, "zh-CN", "perf-news-0"); err != nil {
			b.Fatalf("GetNews: %v", err)
		}
	}
	b.StopTimer()
	if calls := repo.getNewsCalls.Load(); calls != 0 {
		b.Fatalf("expected 0 repository calls for warm reads, got %d", calls)
	}
}

// TestPublicContentCacheServedPerInstance guards the isolation contract: two
// service instances never share repository results, so the cache key space is
// scoped to the owning service.
func TestPublicContentCacheServedPerInstance(t *testing.T) {
	first := NewPublicContentService(newPerfRepo())
	second := NewPublicContentService(newPerfRepo())
	ctx := context.Background()
	if _, err := first.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := second.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		t.Fatalf("second: %v", err)
	}
	hitsFirst, missesFirst, _ := first.publicCaches().newsPage.Stats()
	hitsSecond, missesSecond, _ := second.publicCaches().newsPage.Stats()
	if hitsFirst != 0 || missesFirst != 1 {
		t.Fatalf("first instance stats = hits %d misses %d, want 0/1", hitsFirst, missesFirst)
	}
	if hitsSecond != 0 || missesSecond != 1 {
		t.Fatalf("second instance stats = hits %d misses %d, want 0/1", hitsSecond, missesSecond)
	}
}

// TestPublicContentCacheRetiredOnSnapshotBump proves that publishing content
// (which advances the snapshot version) immediately retires cached pages.
func TestPublicContentCacheRetiredOnSnapshotBump(t *testing.T) {
	repo := newPerfRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()
	if _, err := svc.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if _, err := svc.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		t.Fatalf("cached: %v", err)
	}
	if calls := repo.listNewsCalls.Load(); calls != 1 {
		t.Fatalf("repository calls before invalidation = %d, want 1", calls)
	}

	InvalidatePublicContentCache()
	if _, err := svc.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		t.Fatalf("after invalidation: %v", err)
	}
	if calls := repo.listNewsCalls.Load(); calls != 2 {
		t.Fatalf("repository calls after invalidation = %d, want 2", calls)
	}
}

// TestPublicContentCacheCapsEntries documents the eviction policy precisely:
// this is a bounded map that sweeps stale/expired entries and then drops the
// whole table when it is still saturated. It is NOT an LRU (no recency order is
// tracked), so nothing here should be described as LRU behaviour.
func TestPublicContentCacheCapsEntries(t *testing.T) {
	cache := newPublicContentEntryCache[int](time.Minute, 2, nil)
	version := PublicContentCacheVersion()
	cache.Put("a", version, 1)
	cache.Put("b", version, 2)
	cache.Put("c", version, 3)
	_, _, size := cache.Stats()
	if size > 2 {
		t.Fatalf("cache size = %d, want <= 2", size)
	}
}

// --- TASK-034 concurrency regression (review FAIL: cross-version backfill) ---

// blockingPublicContentRepo freezes the row set at call time and then parks the
// first repository call, so a test can publish (advance the snapshot) while a
// read is still in flight and then let the stale query return.
type blockingPublicContentRepo struct {
	mu            sync.Mutex
	news          []PublicNewsListItem
	newsDetail    []PublicNews
	articles      []PublicArticleListItem
	articleDetail []PublicArticle

	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingRepo() *blockingPublicContentRepo {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	return &blockingPublicContentRepo{
		news: []PublicNewsListItem{{
			Locale: "zh-CN", Slug: "race-news", Title: "old list title", Summary: "Summary",
			SourceName: "Source", PublishedAt: publishedAt, Status: "published",
		}},
		newsDetail: []PublicNews{{
			Locale: "zh-CN", Slug: "race-news", Title: "old detail title", Summary: "Summary",
			SourceName: "Source", BodyHTML: "<p>old</p>", PublishedAt: publishedAt, Status: "published",
		}},
		articles: []PublicArticleListItem{{
			Locale: "zh-CN", Slug: "race-article", Title: "old article list title", Excerpt: "Excerpt",
			PublishedAt: publishedAt, Status: "published",
		}},
		articleDetail: []PublicArticle{{
			Locale: "zh-CN", Slug: "race-article", Title: "old article detail title", Excerpt: "Excerpt",
			BodyHTML: "<p>old</p>", PublishedAt: publishedAt, Status: "published",
		}},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

// park blocks the first repository call until the test closes release.
func (r *blockingPublicContentRepo) park() {
	first := false
	r.once.Do(func() { first = true })
	if !first {
		return
	}
	r.entered <- struct{}{}
	<-r.release
}

func (r *blockingPublicContentRepo) ListPublishedNews(context.Context, string, int, int) (PublicNewsPage, error) {
	r.mu.Lock()
	items := append([]PublicNewsListItem(nil), r.news...)
	r.mu.Unlock()
	r.park()
	return PublicNewsPage{Items: items, Total: len(items)}, nil
}

func (r *blockingPublicContentRepo) GetPublishedNews(context.Context, string, string) (*PublicNews, error) {
	r.mu.Lock()
	var item PublicNews
	if len(r.newsDetail) > 0 {
		item = r.newsDetail[0]
	}
	r.mu.Unlock()
	r.park()
	return &item, nil
}

func (r *blockingPublicContentRepo) ListPublishedArticles(context.Context, string, int, int) (PublicArticlePage, error) {
	r.mu.Lock()
	items := append([]PublicArticleListItem(nil), r.articles...)
	r.mu.Unlock()
	r.park()
	return PublicArticlePage{Items: items, Total: len(items)}, nil
}

func (r *blockingPublicContentRepo) GetPublishedArticle(context.Context, string, string) (*PublicArticle, error) {
	r.mu.Lock()
	var item PublicArticle
	if len(r.articleDetail) > 0 {
		item = r.articleDetail[0]
	}
	r.mu.Unlock()
	r.park()
	return &item, nil
}

func (r *blockingPublicContentRepo) publish(newTitle string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.news) > 0 {
		r.news[0].Title = newTitle
	}
	if len(r.newsDetail) > 0 {
		r.newsDetail[0].Title = newTitle
	}
	if len(r.articles) > 0 {
		r.articles[0].Title = newTitle
	}
	if len(r.articleDetail) > 0 {
		r.articleDetail[0].Title = newTitle
	}
}

// publicCacheTitlesAtVersion lists the titles currently stored under an exact
// snapshot version, which is how the acceptance review detects a value that was
// queried at V but back-filled as if it belonged to V+1.
func publicCacheTitlesAtVersion[T any](c *publicContentEntryCache[T], version uint64, titles func(T) []string) []string {
	c.mu.Lock()
	values := make([]T, 0, len(c.items))
	for _, entry := range c.items {
		if entry.version == version {
			values = append(values, entry.value)
		}
	}
	c.mu.Unlock()
	var out []string
	for _, value := range values {
		out = append(out, titles(value)...)
	}
	return out
}

func assertNoStaleTitleAtVersion[T any](t *testing.T, label string, c *publicContentEntryCache[T], version uint64, stale string, titles func(T) []string) {
	t.Helper()
	for _, title := range publicCacheTitlesAtVersion(c, version, titles) {
		if title == stale {
			t.Fatalf("%s: stale value %q was back-filled under snapshot %d", label, stale, version)
		}
	}
}

func waitRepoEntered(t *testing.T, repo *blockingPublicContentRepo) {
	t.Helper()
	select {
	case <-repo.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("repository call never started")
	}
}

func TestPublicContentCacheDoesNotBackfillAcrossSnapshot_NewsList(t *testing.T) {
	repo := newBlockingRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()

	inflight := make(chan error, 1)
	go func() {
		_, err := svc.ListNews(ctx, "zh-CN", 1, 20)
		inflight <- err
	}()
	waitRepoEntered(t, repo)
	InvalidatePublicContentCache()
	current := PublicContentCacheVersion()
	repo.publish("new list title")
	close(repo.release)
	if err := <-inflight; err != nil {
		t.Fatalf("in-flight list read: %v", err)
	}

	assertNoStaleTitleAtVersion(t, "news list", svc.publicCaches().newsPage, current, "old list title",
		func(page PublicNewsPage) []string {
			titles := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				titles = append(titles, item.Title)
			}
			return titles
		})

	page, err := svc.ListNews(ctx, "zh-CN", 1, 20)
	if err != nil {
		t.Fatalf("fresh list read: %v", err)
	}
	if len(page.Items) == 0 || page.Items[0].Title != "new list title" {
		t.Fatalf("list after publish = %+v, want the new title", page.Items)
	}
}

func TestPublicContentCacheDoesNotBackfillAcrossSnapshot_NewsDetail(t *testing.T) {
	repo := newBlockingRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()

	inflight := make(chan error, 1)
	go func() {
		_, err := svc.GetNews(ctx, "zh-CN", "race-news")
		inflight <- err
	}()
	waitRepoEntered(t, repo)
	InvalidatePublicContentCache()
	current := PublicContentCacheVersion()
	repo.publish("new detail title")
	close(repo.release)
	if err := <-inflight; err != nil {
		t.Fatalf("in-flight detail read: %v", err)
	}

	assertNoStaleTitleAtVersion(t, "news detail", svc.publicCaches().newsDetail, current, "old detail title",
		func(item PublicNews) []string { return []string{item.Title} })

	item, err := svc.GetNews(ctx, "zh-CN", "race-news")
	if err != nil {
		t.Fatalf("fresh detail read: %v", err)
	}
	if item.Title != "new detail title" {
		t.Fatalf("detail after publish = %q, want the new title", item.Title)
	}
}

func TestPublicContentCacheDoesNotBackfillAcrossSnapshot_ArticleList(t *testing.T) {
	repo := newBlockingRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()

	inflight := make(chan error, 1)
	go func() {
		_, err := svc.ListArticles(ctx, "zh-CN", 1, 20)
		inflight <- err
	}()
	waitRepoEntered(t, repo)
	InvalidatePublicContentCache()
	current := PublicContentCacheVersion()
	repo.publish("new article list title")
	close(repo.release)
	if err := <-inflight; err != nil {
		t.Fatalf("in-flight article list read: %v", err)
	}

	assertNoStaleTitleAtVersion(t, "article list", svc.publicCaches().articlePage, current, "old article list title",
		func(page PublicArticlePage) []string {
			titles := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				titles = append(titles, item.Title)
			}
			return titles
		})

	page, err := svc.ListArticles(ctx, "zh-CN", 1, 20)
	if err != nil {
		t.Fatalf("fresh article list read: %v", err)
	}
	if len(page.Items) == 0 || page.Items[0].Title != "new article list title" {
		t.Fatalf("article list after publish = %+v, want the new title", page.Items)
	}
}

func TestPublicContentCacheDoesNotBackfillAcrossSnapshot_ArticleDetail(t *testing.T) {
	repo := newBlockingRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()

	inflight := make(chan error, 1)
	go func() {
		_, err := svc.GetArticle(ctx, "zh-CN", "race-article")
		inflight <- err
	}()
	waitRepoEntered(t, repo)
	InvalidatePublicContentCache()
	current := PublicContentCacheVersion()
	repo.publish("new article detail title")
	close(repo.release)
	if err := <-inflight; err != nil {
		t.Fatalf("in-flight article detail read: %v", err)
	}

	assertNoStaleTitleAtVersion(t, "article detail", svc.publicCaches().articleDetail, current, "old article detail title",
		func(item PublicArticle) []string { return []string{item.Title} })

	item, err := svc.GetArticle(ctx, "zh-CN", "race-article")
	if err != nil {
		t.Fatalf("fresh article detail read: %v", err)
	}
	if item.Title != "new article detail title" {
		t.Fatalf("article detail after publish = %q, want the new title", item.Title)
	}
}

// TestPublicContentCacheReturnsIsolatedCopies makes sure callers cannot poison
// the cache through the slices handed back on a hit (Items/Tags/Alternates).
func TestPublicContentCacheReturnsIsolatedCopies(t *testing.T) {
	repo := newPerfRepo()
	svc := NewPublicContentService(repo)
	ctx := context.Background()

	detail, err := svc.GetNews(ctx, "zh-CN", "perf-news-0")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	detail.Title = "caller mutation"
	detail.Tags = append(detail.Tags, "caller mutation")
	if again, err := svc.GetNews(ctx, "zh-CN", "perf-news-0"); err != nil {
		t.Fatalf("detail again: %v", err)
	} else if again.Title == "caller mutation" || len(again.Tags) != 0 {
		t.Fatalf("detail cache was mutated through the returned value: %+v", again)
	}

	page, err := svc.ListNews(ctx, "zh-CN", 1, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	page.Items[0].Title = "caller mutation"
	page.Items[0].Tags = append(page.Items[0].Tags, "caller mutation")
	if again, err := svc.ListNews(ctx, "zh-CN", 1, 20); err != nil {
		t.Fatalf("list again: %v", err)
	} else if again.Items[0].Title == "caller mutation" || len(again.Items[0].Tags) != 0 {
		t.Fatalf("list cache was mutated through the returned value: %+v", again.Items[0])
	}

	if detailCalls, listCalls := repo.getNewsCalls.Load(), repo.listNewsCalls.Load(); detailCalls != 1 || listCalls != 1 {
		t.Fatalf("expected one repository call per path, got detail=%d list=%d", detailCalls, listCalls)
	}
}

// TestPublicContentEntryCacheExpires drives the TTL path directly with a tiny
// TTL instead of waiting for the production 60s window: an entry must stop
// being served once it expires, so the caller falls back to the repository.
func TestPublicContentEntryCacheExpires(t *testing.T) {
	cache := newPublicContentEntryCache[string](20*time.Millisecond, 8, nil)
	version := PublicContentCacheVersion()
	cache.Put("k", version, "fresh")
	if got, ok := cache.Get("k"); !ok || got != "fresh" {
		t.Fatalf("warm read = (%q, %v), want (fresh, true)", got, ok)
	}
	time.Sleep(40 * time.Millisecond)
	if got, ok := cache.Get("k"); ok {
		t.Fatalf("expired entry still served: %q", got)
	}
	cache.Put("k", version, "reloaded")
	if got, ok := cache.Get("k"); !ok || got != "reloaded" {
		t.Fatalf("read after TTL reload = (%q, %v), want (reloaded, true)", got, ok)
	}
}

// mutablePublicContentRepo serves a single news row whose title can be changed
// mid-test, standing in for a publish between two requests.
type mutablePublicContentRepo struct {
	mu           sync.Mutex
	title        string
	getNewsCalls atomic.Int64
}

func newMutablePublicContentRepo() *mutablePublicContentRepo {
	return &mutablePublicContentRepo{title: "old detail title"}
}

func (r *mutablePublicContentRepo) setNewsTitle(title string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.title = title
}

func (r *mutablePublicContentRepo) currentTitle() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.title
}

func (r *mutablePublicContentRepo) ListPublishedNews(context.Context, string, int, int) (PublicNewsPage, error) {
	now := time.Now().UTC()
	return PublicNewsPage{Items: []PublicNewsListItem{{
		Locale: "zh-CN", Slug: "headline", Title: r.currentTitle(), Summary: "Summary",
		SourceName: "Source", PublishedAt: now, Status: "published",
	}}, Total: 1, Page: 1, PageSize: 20, TotalPages: 1}, nil
}

func (r *mutablePublicContentRepo) GetPublishedNews(context.Context, string, string) (*PublicNews, error) {
	r.getNewsCalls.Add(1)
	now := time.Now().UTC()
	return &PublicNews{
		Locale: "zh-CN", Slug: "headline", Title: r.currentTitle(), Summary: "Summary",
		SourceName: "Source", BodyHTML: "<p>body</p>", PublishedAt: now, Status: "published",
	}, nil
}

func (r *mutablePublicContentRepo) ListPublishedArticles(context.Context, string, int, int) (PublicArticlePage, error) {
	return PublicArticlePage{}, nil
}

func (r *mutablePublicContentRepo) GetPublishedArticle(context.Context, string, string) (*PublicArticle, error) {
	return nil, ErrPublicContentNotFound
}

// TestPublicContentServiceRefetchesAfterPublish checks the write path end to
// end: a publish bumps the snapshot, Get keeps reading the latest snapshot, so
// the next request must miss the cache and observe the new title.
func TestPublicContentServiceRefetchesAfterPublish(t *testing.T) {
	repo := newMutablePublicContentRepo()
	service := NewPublicContentService(repo)

	first, err := service.GetNews(context.Background(), "zh-CN", "headline")
	if err != nil {
		t.Fatalf("first GetNews: %v", err)
	}
	if first.Title != "old detail title" {
		t.Fatalf("first title = %q", first.Title)
	}

	repo.setNewsTitle("new detail title")
	InvalidatePublicContentCache()

	second, err := service.GetNews(context.Background(), "zh-CN", "headline")
	if err != nil {
		t.Fatalf("second GetNews: %v", err)
	}
	if second.Title != "new detail title" {
		t.Fatalf("after publish title = %q, want new detail title", second.Title)
	}
	if got := repo.getNewsCalls.Load(); got != 2 {
		t.Fatalf("repository calls = %d, want 2 (invalidation must force a refetch)", got)
	}
}
