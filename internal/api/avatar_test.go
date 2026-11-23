package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func avatarFixture(t *testing.T) *Server {
	t.Helper()
	t.Chdir(t.TempDir())
	setupTestDB(t)
	if err := database.DB.Create(&database.User{Username: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	return newTestServer()
}

func postAvatar(t *testing.T, s *Server, filename string, contents []byte, want int) string {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/user/avatar", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), "claims", &auth.Claims{Username: "admin"}))
	w := httptest.NewRecorder()
	s.handleUploadAvatar(w, req)
	if w.Code != want {
		t.Fatalf("POST %s: status %d, want %d (%s)", filename, w.Code, want, w.Body)
	}
	var result struct {
		URL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.URL
}

func readAvatar(t *testing.T, s *Server, url string, want int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleGetAvatar(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != want {
		t.Fatalf("GET %s: status %d, want %d", url, w.Code, want)
	}
	return w
}

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"/>`

func TestAvatarSizeAndTypePreservePublishedFile(t *testing.T) {
	s := avatarFixture(t)
	postAvatar(t, s, "bom.svg", []byte("\ufeff"+testSVG), 200)
	limit := 5 << 20
	svg := append([]byte(testSVG), bytes.Repeat([]byte(" "), limit-len(testSVG))...)
	url := postAvatar(t, s, "first.svg", svg, 200)
	postAvatar(t, s, "large.svg", append(svg, ' '), 413)
	postAvatar(t, s, "oversized-request.svg", append(svg, make([]byte, 128<<10)...), 413)
	postAvatar(t, s, "wrong.png", []byte(testSVG), 400)
	postAvatar(t, s, "broken.svg", []byte("<svg>"), 400)
	got := readAvatar(t, s, url, 200)
	if !bytes.Equal(got.Body.Bytes(), svg) || got.Header().Get("Content-Type") != "image/svg+xml" || got.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(got.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("validated image or sandbox headers missing")
	}
	next := postAvatar(t, s, "second.svg", []byte(testSVG), 200)
	if next == url || readAvatar(t, s, "/api/user/avatar", 200).Body.String() != testSVG {
		t.Fatal("current avatar or version did not change")
	}
	readAvatar(t, s, "/api/user/avatar?v=unknown", 404)
}

func TestAvatarMetadataFailureRollsBackAndKeepsOriginal(t *testing.T) {
	s := avatarFixture(t)
	url := postAvatar(t, s, "first.svg", []byte(testSVG), 200)
	if err := database.DB.Exec("CREATE TRIGGER reject_avatar BEFORE UPDATE ON settings WHEN NEW.key='USER_AVATAR_URL' BEGIN SELECT RAISE(ABORT,'fixture'); END").Error; err != nil {
		t.Fatal(err)
	}
	postAvatar(t, s, "second.svg", []byte(strings.ReplaceAll(testSVG, "32", "64")), 500)
	var user database.User
	var setting database.Setting
	if err := database.DB.First(&user, "username = ?", "admin").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.First(&setting, "key = ?", "USER_AVATAR_URL").Error; err != nil {
		t.Fatal(err)
	}
	if user.AvatarURL != url || setting.Value != url || readAvatar(t, s, url, 200).Body.String() != testSVG {
		t.Fatal("failed replacement changed published avatar")
	}
	files, err := os.ReadDir(filepath.Join("data", "uploads", "avatars"))
	if err != nil || len(files) != 1 {
		t.Fatalf("unexpected files after rollback: %d (%v)", len(files), err)
	}
}

func TestAvatarLegacyReadAndReset(t *testing.T) {
	s := avatarFixture(t)
	if err := os.MkdirAll(filepath.Join("data", "uploads"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("data", "uploads", "avatar_admin.svg"), []byte(testSVG), 0600); err != nil {
		t.Fatal(err)
	}
	const legacy = "/api/user/avatar?t=1700000000"
	if readAvatar(t, s, legacy, 200).Body.String() != testSVG {
		t.Fatal("legacy file missing before settings migration")
	}
	if err := database.DB.Save(&database.Setting{Key: "USER_AVATAR_URL", Value: legacy}).Error; err != nil {
		t.Fatal(err)
	}
	if readAvatar(t, s, legacy, 200).Body.String() != testSVG {
		t.Fatal("legacy avatar lost")
	}
	if err := database.DB.Model(&database.Setting{}).Where("key = ?", "USER_AVATAR_URL").Update("value", "").Error; err != nil {
		t.Fatal(err)
	}
	readAvatar(t, s, "/api/user/avatar", 404)
}

func TestAvatarResetReleasesFileAfterSettingsCommit(t *testing.T) {
	s := avatarFixture(t)
	url := postAvatar(t, s, "first.svg", []byte(testSVG), 200)
	w := httptest.NewRecorder()
	s.handleUpdateSettings(w, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"settings":{"USER_AVATAR_URL":""}}`)))
	if w.Code != 200 {
		t.Fatalf("reset: %d", w.Code)
	}
	readAvatar(t, s, url, 404)
	files, err := os.ReadDir(avatarDirectory)
	if err != nil || len(files) != 0 {
		t.Fatalf("retained files after reset: %d (%v)", len(files), err)
	}
}

func TestAvatarRasterFormats(t *testing.T) {
	s := avatarFixture(t)
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, format := range []string{"jpeg", "png", "gif"} {
		var content bytes.Buffer
		var err error
		switch format {
		case "jpeg":
			err = jpeg.Encode(&content, pixel, nil)
		case "png":
			err = png.Encode(&content, pixel)
		case "gif":
			err = gif.Encode(&content, pixel, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		url := postAvatar(t, s, "sample."+format, content.Bytes(), 200)
		if got := readAvatar(t, s, url, 200); !bytes.Equal(got.Body.Bytes(), content.Bytes()) {
			t.Fatalf("%s content changed", format)
		}
	}
	webp, err := base64.StdEncoding.DecodeString("UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoBAAEAAUAmJaACdLoB+AADsAD+8ut//NgVzXPv9//S4P0uD9Lg/9KQAAA=")
	if err != nil {
		t.Fatal(err)
	}
	url := postAvatar(t, s, "sample.webp", webp, 200)
	postAvatar(t, s, "truncated.webp", webp[:20], 400)
	if got := readAvatar(t, s, url, 200); !bytes.Equal(got.Body.Bytes(), webp) {
		t.Fatalf("WebP content changed")
	}
}

func TestAvatarDirectoryFailurePreservesLegacyImage(t *testing.T) {
	s := avatarFixture(t)
	if err := os.MkdirAll("data/uploads", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/uploads/avatar_admin.svg", []byte(testSVG), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(avatarDirectory, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	const url = "/api/user/avatar?t=1700000000"
	if err := database.DB.Save(&database.Setting{Key: avatarSettingKey, Value: url}).Error; err != nil {
		t.Fatal(err)
	}
	postAvatar(t, s, "new.svg", []byte(strings.ReplaceAll(testSVG, "32", "16")), 500)
	if readAvatar(t, s, url, 200).Body.String() != testSVG {
		t.Fatal("file write failure replaced the old avatar")
	}
}
