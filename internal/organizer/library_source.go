package organizer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// LibrarySourceRoot 核对历史媒体文件属于当前类型的媒体库，返回配置目录的绝对路径。
// 库根可以是挂载入口或符号链接，其下只接受通过真实目录访问的非空普通文件。
func (o *TVOrganizer) LibrarySourceRoot(animeType, path string) (string, bool) {
	if path == "" || slices.Contains(strings.Split(filepath.ToSlash(path), "/"), "..") {
		return "", false
	}
	base, err := filepath.Abs(o.getBasePath(animeType))
	if err != nil {
		return "", false
	}
	file, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(base, file)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", false
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return "", false
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", false
	}
	for dir := filepath.Dir(rel); dir != "."; dir = filepath.Dir(dir) {
		info, err := root.Lstat(dir)
		if err != nil || !info.IsDir() {
			return "", false
		}
	}
	return base, true
}
