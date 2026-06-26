package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type pageSettingRepoStub struct {
	values map[string]string
}

func (s *pageSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	if value, ok := s.values[key]; ok {
		return &service.Setting{Key: key, Value: value}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (s *pageSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *pageSettingRepoStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *pageSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *pageSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *pageSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *pageSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestGetPageContentUsesInlineCustomMenuMarkdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const markdown = "# Seedance 视频调用说明\n\n## Seedance\n\n```bash\ncurl https://cc-ai.xyz/v1/video/generations\n```"
	repo := &pageSettingRepoStub{values: map[string]string{
		service.SettingKeyCustomMenuItems: `[{"id":"seedance_video_guide","label":"seedace视频调用说明","url":"md:seedance-video-guide","content_md":` + strconv.Quote(markdown) + `,"visibility":"user","sort_order":1}]`,
	}}
	handler := NewPageHandler(t.TempDir(), service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/pages/:slug", func(c *gin.Context) {
		c.Set("user_role", "user")
		handler.GetPageContent(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pages/seedance-video-guide", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != markdown {
		t.Fatalf("body = %q, want %q", got, markdown)
	}
}

func TestGetPageContentBlocksAdminInlineMarkdownForUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const markdown = "# Admin Only"
	repo := &pageSettingRepoStub{values: map[string]string{
		service.SettingKeyCustomMenuItems: `[{"id":"admin_guide","label":"Admin","url":"md:admin-guide","content_md":` + strconv.Quote(markdown) + `,"visibility":"admin","sort_order":1}]`,
	}}
	handler := NewPageHandler(t.TempDir(), service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/pages/:slug", func(c *gin.Context) {
		c.Set("user_role", "user")
		handler.GetPageContent(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pages/admin-guide", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), markdown) {
		t.Fatalf("admin markdown leaked in response: %s", rec.Body.String())
	}
}

func TestCleanPageImageRelativePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "single filename", in: "logo.png", want: "logo.png", ok: true},
		{name: "nested path", in: "images/logo.png", want: filepath.Join("images", "logo.png"), ok: true},
		{name: "dot prefix", in: "./logo.png", want: "logo.png", ok: true},
		{name: "url escaped slash", in: "images%2Flogo.png", want: filepath.Join("images", "logo.png"), ok: true},
		{name: "parent traversal", in: "../secret.png", ok: false},
		{name: "encoded parent traversal", in: "%2e%2e/secret.png", ok: false},
		{name: "backslash traversal", in: `images\secret.png`, ok: false},
		{name: "absolute path", in: "/etc/passwd", ok: false},
		{name: "encoded absolute path", in: "%2fetc/passwd", ok: false},
		{name: "encoded nul byte", in: "logo.png%00", ok: false},
		{name: "invalid escape", in: "logo.png%zz", ok: false},
		{name: "empty path", in: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanPageImageRelativePath(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePageImagePath(t *testing.T) {
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	base := filepath.Join(pagesDir, "guide")
	if err := os.MkdirAll(filepath.Join(base, "images"), 0755); err != nil {
		t.Fatalf("create images dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "logo.png"), []byte("fake"), 0644); err != nil {
		t.Fatalf("create direct image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "images", "logo.png"), []byte("fake"), 0644); err != nil {
		t.Fatalf("create image: %v", err)
	}

	got, ok := resolvePageImagePath(pagesDir, base, "logo.png")
	if !ok {
		t.Fatal("expected direct image path to be accepted")
	}
	want := mustEvalSymlinks(t, filepath.Join(base, "logo.png"))
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	got, ok = resolvePageImagePath(pagesDir, base, "images/logo.png")
	if !ok {
		t.Fatal("expected nested image path to be accepted")
	}
	want = mustEvalSymlinks(t, filepath.Join(base, "images", "logo.png"))
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	if got, ok := resolvePageImagePath(pagesDir, base, "../guide.md"); ok {
		t.Fatalf("expected traversal to be rejected, got %q", got)
	}
}

func TestResolvePageImagePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	base := filepath.Join(pagesDir, "guide")
	outside := filepath.Join(root, "outside")

	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("create page dir: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("secret"), 0644); err != nil {
		t.Fatalf("create outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "images")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if got, ok := resolvePageImagePath(pagesDir, base, "images/secret.png"); ok {
		t.Fatalf("expected symlink escape to be rejected, got %q", got)
	}
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()

	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks for %q: %v", path, err)
	}
	return realPath
}
