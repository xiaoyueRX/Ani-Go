package core

import (
	"path/filepath"
	"strings"
)

var mediaServerReplacer = strings.NewReplacer(
	":", " - ",
	"?", "？",
	"*", " ",
	"\"", "'",
	"<", "（",
	">", "）",
	"|", " - ",
)

// SanitizePathForMediaServer 清洗整理路径中的跨平台非法字符与多余空格，并保留绝对路径根前缀
func SanitizePathForMediaServer(p string) string {
	if strings.TrimSpace(p) == "" {
		return p
	}
	clean := filepath.Clean(p)
	if clean == "." || clean == string(filepath.Separator) {
		return clean
	}

	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	hasRootSep := strings.HasPrefix(rest, string(filepath.Separator))
	trimmed := strings.TrimPrefix(rest, string(filepath.Separator))
	if trimmed == "" {
		return clean
	}

	rawParts := strings.Split(trimmed, string(filepath.Separator))
	cleanedParts := make([]string, 0, len(rawParts))
	for _, seg := range rawParts {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			cleanedParts = append(cleanedParts, seg)
			continue
		}
		s := mediaServerReplacer.Replace(seg)
		fields := strings.Fields(s)
		if len(fields) > 0 {
			s = strings.Join(fields, " ")
		} else {
			s = ""
		}
		// 移除文件名或目录末尾的空格与句点（Windows 规范）
		s = strings.TrimRight(s, " .")
		if s == "" {
			s = "_"
		}
		cleanedParts = append(cleanedParts, s)
	}

	joined := filepath.Join(cleanedParts...)
	if hasRootSep {
		return vol + string(filepath.Separator) + joined
	}
	if vol != "" {
		return vol + joined
	}
	return joined
}
