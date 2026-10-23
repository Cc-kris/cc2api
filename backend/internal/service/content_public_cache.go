package service

import "sync/atomic"

var publicContentCacheInvalidator atomic.Value

// RegisterPublicContentCacheInvalidator connects content mutations to the
// renderer's public HTML cache without coupling the service package to HTTP.
func RegisterPublicContentCacheInvalidator(fn func()) {
	if fn == nil {
		return
	}
	publicContentCacheInvalidator.Store(fn)
}

// InvalidatePublicContentCache invalidates published HTML/list/sitemap output
// after a successful content mutation. It is intentionally safe when the
// renderer is not installed (for CLI/tests).
func InvalidatePublicContentCache() {
	if value := publicContentCacheInvalidator.Load(); value != nil {
		if fn, ok := value.(func()); ok {
			fn()
		}
	}
}
