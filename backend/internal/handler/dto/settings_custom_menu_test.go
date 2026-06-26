package dto

import "testing"

func TestParseUserVisibleMenuItemsStripsMarkdownContent(t *testing.T) {
	raw := `[
		{"id":"seedance_video_guide","label":"seedace视频调用说明","url":"md:seedance-video-guide","page_slug":"seedance-video-guide","content_md":"# private content","visibility":"user","sort_order":1},
		{"id":"admin_page","label":"Admin","url":"md:admin","content_md":"# admin content","visibility":"admin","sort_order":2}
	]`

	items := ParseUserVisibleMenuItems(raw)
	if len(items) != 1 {
		t.Fatalf("expected 1 user-visible item, got %d", len(items))
	}
	if items[0].ID != "seedance_video_guide" {
		t.Fatalf("unexpected item id: %s", items[0].ID)
	}
	if items[0].PageSlug != "seedance-video-guide" {
		t.Fatalf("unexpected page slug: %s", items[0].PageSlug)
	}
	if items[0].ContentMD != "" {
		t.Fatalf("expected markdown content to be stripped, got %q", items[0].ContentMD)
	}
}
