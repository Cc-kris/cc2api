package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type contentNewsWorkerSourceStub struct {
	source   *ContentNewsSource
	due      []ContentNewsSource
	recorded bool
	lastErr  *string
}

type contentNewsWorkerQueuedRunStub struct {
	contentNewsWorkerRunStub
	queued  []*ContentFetchRun
	claimed map[int64]bool
	updated []*ContentFetchRun
}

func (s *contentNewsWorkerQueuedRunStub) ListQueuedFetchRuns(context.Context, int) ([]ContentFetchRun, error) {
	out := make([]ContentFetchRun, 0, len(s.queued))
	for _, run := range s.queued {
		out = append(out, *run)
	}
	return out, nil
}
func (s *contentNewsWorkerQueuedRunStub) ClaimFetchRun(_ context.Context, id int64, now time.Time) (*ContentFetchRun, error) {
	if s.claimed == nil {
		s.claimed = map[int64]bool{}
	}
	if s.claimed[id] {
		return nil, nil
	}
	for _, run := range s.queued {
		if run.ID == id {
			s.claimed[id] = true
			run.Status = domain.ContentFetchRunning
			run.StartedAt = &now
			return run, nil
		}
	}
	return nil, nil
}

func (s *contentNewsWorkerQueuedRunStub) UpdateFetchRun(_ context.Context, run *ContentFetchRun) (*ContentFetchRun, error) {
	s.updated = append(s.updated, run)
	return run, nil
}

func (s *contentNewsWorkerSourceStub) GetNewsSource(context.Context, int64, bool) (*ContentNewsSource, error) {
	return s.source, nil
}
func (s *contentNewsWorkerSourceStub) ListDueNewsSources(context.Context, time.Time, int) ([]ContentNewsSource, error) {
	return s.due, nil
}
func (s *contentNewsWorkerSourceStub) RecordNewsSourceFetch(_ context.Context, _ int64, _ time.Time, _ *time.Time, err *string) error {
	s.recorded, s.lastErr = true, err
	return nil
}

type contentNewsWorkerRunStub struct{ runs []*ContentFetchRun }

func (s *contentNewsWorkerRunStub) CreateFetchRun(_ context.Context, run *ContentFetchRun) (*ContentFetchRun, error) {
	run.ID = int64(len(s.runs) + 1)
	s.runs = append(s.runs, run)
	return run, nil
}
func (s *contentNewsWorkerRunStub) UpdateFetchRun(_ context.Context, run *ContentFetchRun) (*ContentFetchRun, error) {
	return run, nil
}

type contentNewsWorkerItemStub struct {
	saved         []*ContentNewsItem
	autoPublished int
	upsertErr     error
}

func (s *contentNewsWorkerItemStub) UpsertFetchedNewsItem(_ context.Context, item *ContentNewsItem) (ContentNewsFetchItemResult, error) {
	if s.upsertErr != nil {
		return ContentNewsFetchItemResult{}, s.upsertErr
	}
	s.saved = append(s.saved, item)
	return ContentNewsFetchItemResult{Created: true}, nil
}
func (s *contentNewsWorkerItemStub) CountAutoPublishedNews(context.Context, time.Time, time.Time) (int, error) {
	return s.autoPublished, nil
}
func (s *contentNewsWorkerItemStub) CountFetchedNews(context.Context, time.Time, time.Time) (int, error) {
	return len(s.saved), nil
}

type contentNewsWorkerPolicyStub struct {
	policy *ContentNewsWorkerPolicy
	err    error
}

func (s *contentNewsWorkerPolicyStub) GetContentNewsWorkerPolicy(context.Context) (*ContentNewsWorkerPolicy, error) {
	return s.policy, s.err
}

type contentNewsFetcherFunc func(context.Context, string) ([]byte, string, error)

func (f contentNewsFetcherFunc) Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	return f(ctx, raw)
}

type contentNewsArticleFetcherStub struct {
	feed    []byte
	article []byte
}

func (f contentNewsArticleFetcherStub) Fetch(context.Context, string) ([]byte, string, error) {
	return f.feed, "https://feed.example/rss", nil
}

func (f contentNewsArticleFetcherStub) FetchArticle(context.Context, string) ([]byte, string, error) {
	return f.article, "https://news.example/article", nil
}

func workerAuthorizedSource() *ContentNewsSource {
	u, note, robots := "https://feed.example/rss", "licensed feed", "allowed"
	now := time.Now().UTC()
	return &ContentNewsSource{ID: 7, Name: "CCAI News", SourceType: domain.ContentNewsSourceRSS, URL: &u, Language: "zh-CN", Enabled: true, IntervalMinutes: 15, RateLimitPerMinute: 60, TermsConfirmedAt: &now, AuthorizationNote: &note, RobotsPolicy: &robots}
}

func TestContentNewsWorkerRunsFeedAndAutoPublishesCompleteContent(t *testing.T) {
	source := workerAuthorizedSource()
	items := &contentNewsWorkerItemStub{}
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: source}, &contentNewsWorkerRunStub{}, items,
		&contentNewsWorkerPolicyStub{policy: &ContentNewsWorkerPolicy{Enabled: true, PublishMode: domain.ContentAutomationAutoPublish, TargetLocales: []string{"zh-CN"}}},
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) {
			return []byte(`<rss><channel><item><guid>abc</guid><link>https://news.example/a</link><title>CCAI release</title><description>short intro</description><encoded><![CDATA[<p>Complete <strong>news</strong><a href="https://source.example"> link</a><script>bad()</script></p>]]></encoded><pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate></item></channel></rss>`), "https://feed.example/rss", nil
		}))
	fixed := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return fixed }
	run, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if err != nil {
		t.Fatalf("RunSource: %v", err)
	}
	if run.Status != domain.ContentFetchSucceeded || run.CreatedCount != 1 {
		t.Fatalf("unexpected run: %#v", run)
	}
	if len(items.saved) != 1 {
		t.Fatalf("saved=%d", len(items.saved))
	}
	item := items.saved[0]
	if item.Status != domain.ContentStatusPublished || item.BodyHTML == nil || strings.Contains(*item.BodyHTML, "href=") || strings.Contains(*item.BodyHTML, "script") {
		t.Fatalf("unsafe or unpublished item: %#v", item)
	}
	if item.BodyText == nil || !strings.Contains(*item.BodyText, "Complete news link") || item.ContentHash == nil || item.Slug == "" {
		t.Fatalf("missing normalized fields: %#v", item)
	}
}

func TestContentNewsWorkerPersistsSummaryOnlyFeedForReview(t *testing.T) {
	source := workerAuthorizedSource()
	items := &contentNewsWorkerItemStub{}
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: source}, &contentNewsWorkerRunStub{}, items,
		&contentNewsWorkerPolicyStub{policy: &ContentNewsWorkerPolicy{Enabled: true, PublishMode: domain.ContentAutomationAutoPublish, TargetLocales: []string{"zh-CN"}}},
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) {
			return []byte(`<rss><channel><item><guid>a</guid><title>Only summary</title><link>https://news.example/a</link><description>Preview only</description></item></channel></rss>`), "https://feed.example", nil
		}))
	run, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if !errors.Is(err, ErrContentNewsArticleIncomplete) || run == nil || len(items.saved) != 1 {
		t.Fatalf("summary feed must be persisted for review: run=%#v saved=%#v err=%v", run, items.saved, err)
	}
	if items.saved[0].Status != domain.ContentStatusPendingReview || items.saved[0].BodyHTML != nil || items.saved[0].BodyText != nil {
		t.Fatalf("summary feed should be pending review without body: %#v", items.saved[0])
	}
}

func TestContentNewsWorkerFetchesAndCleansFullArticleWhenFeedHasSummaryOnly(t *testing.T) {
	source := workerAuthorizedSource()
	items := &contentNewsWorkerItemStub{}
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: source}, &contentNewsWorkerRunStub{}, items,
		&contentNewsWorkerPolicyStub{policy: &ContentNewsWorkerPolicy{Enabled: true, PublishMode: domain.ContentAutomationAutoPublish, TargetLocales: []string{"zh-CN"}}},
		contentNewsArticleFetcherStub{
			feed:    []byte(`<rss><channel><item><guid>a</guid><title>Article</title><link>https://news.example/a</link><description>Preview</description></item></channel></rss>`),
			article: []byte(`<html><body><nav>menu</nav><article><h1>Full story</h1><p>Original text <a href="https://evil.example">source</a></p><iframe src="https://evil.example/frame"></iframe></article><footer>footer</footer></body></html>`),
		})
	_, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if err != nil || len(items.saved) != 1 || items.saved[0].Status != domain.ContentStatusPublished {
		t.Fatalf("article fetch did not publish: saved=%#v err=%v", items.saved, err)
	}
	if items.saved[0].BodyText == nil || !strings.Contains(*items.saved[0].BodyText, "Full story") || strings.Contains(*items.saved[0].BodyHTML, "iframe") || strings.Contains(*items.saved[0].BodyHTML, "href=") {
		t.Fatalf("unsafe/incomplete article body: %#v", items.saved[0])
	}
}

func TestContentNewsWorkerRecordsFailedRun(t *testing.T) {
	source := workerAuthorizedSource()
	sources := &contentNewsWorkerSourceStub{source: source}
	worker := NewContentNewsWorker(sources, &contentNewsWorkerRunStub{}, &contentNewsWorkerItemStub{}, nil,
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) { return nil, "", errors.New("upstream\nsecret") }))
	run, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if err == nil || run == nil || run.Status != domain.ContentFetchFailed || !sources.recorded || sources.lastErr == nil || strings.Contains(*sources.lastErr, "\n") {
		t.Fatalf("failed state not persisted: run=%#v err=%v source=%#v", run, err, sources)
	}
}

func TestContentNewsWorkerQueuedRunDoesNotRemainRunningWhenSourceCannotRun(t *testing.T) {
	queued := &ContentFetchRun{ID: 41, SourceID: 7, TriggerType: domain.ContentFetchTriggerCron, Status: domain.ContentFetchQueued, TaskKey: "queued-41"}
	runs := &contentNewsWorkerQueuedRunStub{queued: []*ContentFetchRun{queued}}
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: nil}, runs, &contentNewsWorkerItemStub{}, nil,
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) { return nil, "", nil }))
	if err := worker.ProcessQueuedRuns(context.Background(), 1); err == nil {
		t.Fatal("expected queued run failure")
	}
	if len(runs.updated) != 1 || runs.updated[0].Status != domain.ContentFetchFailed || runs.updated[0].FinishedAt == nil {
		t.Fatalf("queued run was not finalized: %#v", runs.updated)
	}
}

func TestParseContentNewsFeedAtomAndRSS(t *testing.T) {
	atom, err := ParseContentNewsFeed([]byte(`<feed><entry><id>atom-id</id><title>Atom</title><link rel="alternate" href="https://news.example/a"/><content><![CDATA[<p>full body</p>]]></content></entry></feed>`), domain.ContentNewsSourceAtom)
	if err != nil || len(atom) != 1 || !atom[0].BodyComplete || atom[0].CanonicalURL == "" {
		t.Fatalf("atom parse failed: %#v %v", atom, err)
	}
	rss, err := ParseContentNewsFeed([]byte(`<rss><channel><item><guid>rss-id</guid><title>RSS</title><link>https://news.example/r</link><description>desc</description></item></channel></rss>`), domain.ContentNewsSourceRSS)
	if err != nil || len(rss) != 1 || rss[0].BodyComplete {
		t.Fatalf("rss parse failed: %#v %v", rss, err)
	}
}

func TestParseContentNewsFeedPublicJSON(t *testing.T) {
	entries, err := ParseContentNewsFeed([]byte(`{"items":[{"id":"social-1","title":"Social update","summary":"summary","body":"<p>full body</p>","url":"https://social.example/post/1","source":"Social"}]}`), domain.ContentNewsSourcePublicJSON)
	if err != nil || len(entries) != 1 || !entries[0].BodyComplete || entries[0].SourceName != "Social" {
		t.Fatalf("public json parse failed: %#v %v", entries, err)
	}
}

func TestParseContentNewsFeedPublicJSONWithMapping(t *testing.T) {
	body := []byte(`{"data":[{"meta":{"headline":"Mapped title","id":"m-1"},"article":{"text":"<p>Mapped body</p>","href":"https://example.test/m-1"},"published":"2026-09-11T08:00:00Z"}]}`)
	entries, err := ParseContentNewsFeedWithMapping(body, domain.ContentNewsSourcePublicJSON, map[string]string{
		"title": "$.meta.headline", "id": "$.meta.id", "body": "$.article.text", "url": "$.article.href", "published_at": "$.published",
	})
	if err != nil || len(entries) != 1 || entries[0].Title != "Mapped title" || entries[0].CanonicalURL == "" || !entries[0].BodyComplete {
		t.Fatalf("mapped public json parse failed: %#v %v", entries, err)
	}
}

func TestExtractContentNewsArticleHTMLPrefersArticle(t *testing.T) {
	html, text := ExtractContentNewsArticleHTML([]byte(`<html><body><nav>ignore</nav><main><p>keep me</p><script>bad</script></main></body></html>`))
	if !strings.Contains(html, "keep me") || strings.Contains(html, "ignore") || strings.Contains(html, "script") || text != "keep me" {
		t.Fatalf("unexpected article extraction: html=%q text=%q", html, text)
	}
}

func TestExtractContentNewsArticleHTMLDropsLayoutElementsOnBodyFallback(t *testing.T) {
	html, text := ExtractContentNewsArticleHTML([]byte(`<html><body><nav>ignore nav</nav><p>keep body</p><footer>ignore footer</footer></body></html>`))
	if !strings.Contains(html, "keep body") || strings.Contains(html, "ignore") || text != "keep body" {
		t.Fatalf("layout text leaked: html=%q text=%q", html, text)
	}
}

func TestContentNewsWorkerRejectsEmptyFeed(t *testing.T) {
	source := workerAuthorizedSource()
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: source}, &contentNewsWorkerRunStub{}, &contentNewsWorkerItemStub{}, nil,
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) {
			return []byte(`<rss><channel></channel></rss>`), "https://feed.example/rss", nil
		}))
	run, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if !errors.Is(err, ErrContentNewsFeedEmpty) || run == nil || run.Status != domain.ContentFetchFailed {
		t.Fatalf("empty feed should fail: run=%#v err=%v", run, err)
	}
}

func TestContentNewsWorkerEnforcesConfiguredBatchPublishLimit(t *testing.T) {
	source := workerAuthorizedSource()
	items := &contentNewsWorkerItemStub{}
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: source}, &contentNewsWorkerRunStub{}, items,
		&contentNewsWorkerPolicyStub{policy: &ContentNewsWorkerPolicy{Enabled: true, PublishMode: domain.ContentAutomationAutoPublish, DailyPublishLimit: 1, TargetLocales: []string{"zh-CN"}}},
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) {
			return []byte(`<rss><channel><item><guid>a</guid><link>https://news.example/a</link><title>First</title><description>first</description><encoded><![CDATA[<p>first full</p>]]></encoded></item><item><guid>b</guid><link>https://news.example/b</link><title>Second</title><description>second</description><encoded><![CDATA[<p>second full</p>]]></encoded></item></channel></rss>`), "https://feed.example/rss", nil
		}))
	_, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if err != nil || len(items.saved) != 2 {
		t.Fatalf("worker failed: saved=%d err=%v", len(items.saved), err)
	}
	if items.saved[0].Status != domain.ContentStatusPublished || items.saved[1].Status != domain.ContentStatusDraft {
		t.Fatalf("configured batch limit not enforced: %#v", items.saved)
	}
}

func TestContentNewsWorkerEnforcesConfiguredDailyFetchLimit(t *testing.T) {
	source := workerAuthorizedSource()
	items := &contentNewsWorkerItemStub{}
	worker := NewContentNewsWorker(&contentNewsWorkerSourceStub{source: source}, &contentNewsWorkerRunStub{}, items,
		&contentNewsWorkerPolicyStub{policy: &ContentNewsWorkerPolicy{Enabled: true, PublishMode: domain.ContentAutomationManualReview, DailyFetchLimit: 1, TargetLocales: []string{"zh-CN"}}},
		contentNewsFetcherFunc(func(context.Context, string) ([]byte, string, error) {
			return []byte(`<rss><channel><item><guid>a</guid><link>https://news.example/a</link><title>First</title><description>first</description><encoded><![CDATA[<p>first full</p>]]></encoded></item><item><guid>b</guid><link>https://news.example/b</link><title>Second</title><description>second</description><encoded><![CDATA[<p>second full</p>]]></encoded></item></channel></rss>`), "https://feed.example/rss", nil
		}))
	run, err := worker.RunSource(context.Background(), source.ID, domain.ContentFetchTriggerManual)
	if err != nil || run.CreatedCount != 1 || len(items.saved) != 1 {
		t.Fatalf("configured daily fetch limit not enforced: run=%#v saved=%d err=%v", run, len(items.saved), err)
	}
}

func TestValidateContentNewsFetchURLBlocksPrivateAndDNS(t *testing.T) {
	resolver := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }
	if err := ValidateContentNewsFetchURL(context.Background(), "http://public.example/feed", resolver); !errors.Is(err, ErrContentNewsFetchURLBlocked) {
		t.Fatalf("expected dns block, got %v", err)
	}
	if err := ValidateContentNewsFetchURL(context.Background(), "http://127.0.0.1/feed", resolver); !errors.Is(err, ErrContentNewsFetchURLBlocked) {
		t.Fatalf("expected literal block, got %v", err)
	}
	public := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	if err := ValidateContentNewsFetchURL(context.Background(), "ftp://public.example/feed", public); !errors.Is(err, ErrContentNewsFetchURLInvalid) {
		t.Fatalf("expected scheme block, got %v", err)
	}
	if err := ValidateContentNewsFetchURL(context.Background(), "https://user@public.example/feed", public); !errors.Is(err, ErrContentNewsFetchURLInvalid) {
		t.Fatalf("expected userinfo block, got %v", err)
	}
}

func TestSafeContentNewsHTTPFetcherValidatesRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
	}))
	defer server.Close()
	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+port)
	}
	resolver := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	fetcher := NewSafeContentNewsHTTPFetcher(&http.Client{Transport: transport}, resolver)
	_, _, err := fetcher.Fetch(context.Background(), "http://feed.example:"+port+"/rss")
	if !errors.Is(err, ErrContentNewsFetchURLBlocked) {
		t.Fatalf("redirect should be blocked, got %v", err)
	}
}

func TestSafeContentNewsHTTPFetcherRetriesTransientStatus(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		status := http.StatusOK
		body := "<rss/>"
		if calls.Add(1) < 3 {
			status = http.StatusBadGateway
			body = "busy"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Request: &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}}, nil
	})}
	f := NewSafeContentNewsHTTPFetcher(client, func(context.Context, string) ([]net.IP, error) { return []net.IP{{93, 184, 216, 34}}, nil })
	body, _, err := f.Fetch(context.Background(), "https://example.com/feed")
	if err != nil || string(body) != "<rss/>" || calls.Load() != 3 {
		t.Fatalf("retry failed: calls=%d body=%q err=%v", calls.Load(), body, err)
	}
}

func TestSafeContentNewsHTTPFetcherRetriesBodyReadFailure(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempt := calls.Add(1)
		body := io.Reader(strings.NewReader("<rss/>"))
		if attempt < 3 {
			body = failingReader{}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Request: &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}}, nil
	})}
	f := NewSafeContentNewsHTTPFetcher(client, func(context.Context, string) ([]net.IP, error) { return []net.IP{{93, 184, 216, 34}}, nil })
	body, _, err := f.Fetch(context.Background(), "https://example.com/feed")
	if err != nil || string(body) != "<rss/>" || calls.Load() != 3 {
		t.Fatalf("body read retry failed: calls=%d body=%q err=%v", calls.Load(), body, err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("temporary body read failure") }
