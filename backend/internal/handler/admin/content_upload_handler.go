package admin

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// maxContentImageBytes caps a single content-center image upload.
const maxContentImageBytes = 5 << 20 // 5 MiB

// contentImageMaxPixels is not enforced here (no decoder round-trip); the size
// cap above bounds the work the renderer may face.

// ContentUploadHandler stores images used by the content center (article
// bodies and covers) on local disk and returns a same-origin URL that the
// public renderer's image allowlist accepts (/media/content/...).
type ContentUploadHandler struct {
	dir string
}

// NewContentUploadHandler builds the handler rooted at <data_dir>/media/content.
func NewContentUploadHandler(cfg *config.Config) *ContentUploadHandler {
	dir := ""
	if cfg != nil {
		base := strings.TrimSpace(cfg.Pricing.DataDir)
		if base == "" {
			base = "./data"
		}
		dir = filepath.Join(base, "media", "content")
	}
	if dir == "" {
		dir = filepath.Join("data", "media", "content")
	}
	return &ContentUploadHandler{dir: dir}
}

// UploadDir exposes the resolved directory (used by router static serving).
func (h *ContentUploadHandler) UploadDir() string {
	if h == nil {
		return ""
	}
	return h.dir
}

// UploadImage handles POST /api/v1/admin/content/upload (multipart field file).
func (h *ContentUploadHandler) UploadImage(c *gin.Context) {
	if h == nil || strings.TrimSpace(h.dir) == "" {
		response.ErrorFrom(c, errors.New("content upload storage unavailable"))
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "缺少上传文件字段 file")
		return
	}
	if fileHeader.Size <= 0 {
		response.BadRequest(c, "上传文件为空")
		return
	}
	if fileHeader.Size > maxContentImageBytes {
		response.BadRequest(c, "图片大小不能超过 5MB")
		return
	}
	ext, err := imageFileExt(fileHeader)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	name := randomUploadName() + ext
	dst := filepath.Join(h.dir, name)
	if err := saveUploadedFile(fileHeader, dst); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"url":      "/media/content/" + name,
		"filename": name,
		"size":     fileHeader.Size,
	})
}

func saveUploadedFile(fh *multipart.FileHeader, dst string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		_ = os.Remove(tmpName)
	}()
	written, err := io.Copy(tmp, io.LimitReader(src, maxContentImageBytes+1))
	if err != nil {
		return err
	}
	if written > maxContentImageBytes {
		return errors.New("图片大小不能超过 5MB")
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

// imageFileExt sniffs the uploaded content and returns the canonical extension.
// SVG is rejected on purpose: it can carry script and the file is served from
// the app origin.
func imageFileExt(fh *multipart.FileHeader) (string, error) {
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	detected := http.DetectContentType(head[:n])
	switch detected {
	case "image/jpeg":
		return ".jpg", nil
	case "image/png":
		return ".png", nil
	case "image/gif":
		return ".gif", nil
	case "image/webp":
		return ".webp", nil
	case "image/bmp":
		return ".bmp", nil
	}
	return "", errors.New("仅支持 JPEG/PNG/GIF/WEBP/BMP 图片")
}

func randomUploadName() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failure would break uniqueness; fall back to a timestamp
		// so the upload still gets a distinct, non-guessable-enough name.
		return fmt.Sprintf("upload-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
