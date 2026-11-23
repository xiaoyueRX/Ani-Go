package api

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

const (
	maxAvatarBytes          = 5 << 20
	avatarMultipartOverhead = 64 << 10
	avatarDirectory         = "data/uploads/avatars"
	avatarURLPrefix         = "/api/user/avatar?v="
	avatarSettingKey        = "USER_AVATAR_URL"
)

var avatarVersion = regexp.MustCompile(`^avatar-[0-9]+\.(png|jpg|jpeg|webp|gif|svg)$`)

var avatarMediaTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
}

func isAvatarImage(data []byte, ext string) bool {
	if ext == ".svg" {
		decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\ufeff"))))
		depth := 0
		seenRoot := false
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return seenRoot && depth == 0
			}
			if err != nil {
				return false
			}
			switch node := token.(type) {
			case xml.StartElement:
				if depth == 0 {
					if seenRoot || node.Name.Local != "svg" || (node.Name.Space != "" && node.Name.Space != "http://www.w3.org/2000/svg") {
						return false
					}
					seenRoot = true
				}
				depth++
			case xml.EndElement:
				depth--
			case xml.CharData:
				if depth == 0 && len(bytes.TrimSpace(node)) != 0 {
					return false
				}
			case xml.Directive:
				return false
			}
		}
	}
	if http.DetectContentType(data) != avatarMediaTypes[ext] {
		return false
	}
	if ext == ".webp" {
		if len(data) < 20 || binary.LittleEndian.Uint32(data[4:8]) != uint32(len(data)-8) {
			return false
		}
		chunkSize := binary.LittleEndian.Uint32(data[16:20])
		if chunkSize == 0 || chunkSize > uint32(len(data)-20) {
			return false
		}
		switch string(data[12:16]) {
		case "VP8 ", "VP8L", "VP8X":
			return true
		}
		return false
	}
	_, _, err := image.DecodeConfig(bytes.NewReader(data))
	return err == nil
}

func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value("claims").(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "未登录或登录已过期"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+avatarMultipartOverhead)
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "头像不能超过 5 MiB"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的头像上传请求"})
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "未找到上传的文件，字段名应为 avatar"})
		return
	}
	defer file.Close()
	if header.Size > maxAvatarBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "头像不能超过 5 MiB"})
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "读取头像失败"})
		return
	}
	if !isAvatarImage(data, ext) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "文件内容与格式不匹配，仅支持 PNG、JPEG、WebP、GIF 或 SVG 图片"})
		return
	}
	if err := os.MkdirAll(avatarDirectory, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建头像目录失败"})
		return
	}
	dest, err := os.CreateTemp(avatarDirectory, "avatar-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建头像文件失败"})
		return
	}
	url := avatarURLPrefix + filepath.Base(dest.Name())
	_, writeErr := dest.Write(data)
	closeErr := dest.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(dest.Name())
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "写入头像失败"})
		return
	}

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	var user database.User
	err = database.DB.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("id", "avatar_url").Where("username = ?", claims.Username).First(&user).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.User{}).Where("id = ?", user.ID).Update("avatar_url", url).Error; err != nil {
			return err
		}
		return tx.Save(&database.Setting{Key: avatarSettingKey, Value: url}).Error
	})
	if err != nil {
		_ = os.Remove(dest.Name())
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "保存头像配置失败"})
		return
	}
	removeUploadedAvatar(user.AvatarURL)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "avatar_url": url, "message": "头像上传成功"})
}

// 只清理本服务生成的头像文件，外部头像 URL 与旧版文件名不参与删除。
func removeUploadedAvatar(url string) {
	if name, ok := strings.CutPrefix(url, avatarURLPrefix); ok && avatarVersion.MatchString(name) {
		_ = os.Remove(filepath.Join(avatarDirectory, name))
	}
}

func (s *Server) handleGetAvatar(w http.ResponseWriter, r *http.Request) {
	var setting database.Setting
	if err := database.DB.WithContext(r.Context()).Where("key = ?", avatarSettingKey).Find(&setting).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取头像配置失败"})
		return
	}
	requested, versioned := r.URL.Query()["v"]
	version, stored := strings.CutPrefix(setting.Value, avatarURLPrefix)
	if versioned && (!stored || len(requested) != 1 || requested[0] != version) {
		http.NotFound(w, r)
		return
	}

	var path string
	if stored && avatarVersion.MatchString(version) {
		path = filepath.Join(avatarDirectory, version)
	} else if setting.Key == "" || setting.Value == "/api/user/avatar" || strings.HasPrefix(setting.Value, "/api/user/avatar?t=") {
		// 升级前上传的头像采用固定文件名，旧缓存地址仍可读取。
		matches, _ := filepath.Glob("data/uploads/avatar_*")
		for i := len(matches) - 1; i >= 0; i-- {
			if avatarMediaTypes[strings.ToLower(filepath.Ext(matches[i]))] != "" {
				path = matches[i]
				break
			}
		}
	}
	if path == "" {
		http.NotFound(w, r)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", avatarMediaTypes[strings.ToLower(filepath.Ext(path))])
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	if versioned {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}
