package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"golang.org/x/mod/semver"
)

// EnvironmentType 部署环境类型
type EnvironmentType string

const (
	EnvBinary EnvironmentType = "binary" // 独立二进制运行
	EnvDocker EnvironmentType = "docker" // Docker 容器内运行
)

// UpdateInfo 更新检测信息
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	PublishedAt    string `json:"published_at"`
	Environment    string `json:"environment"`     // binary 或 docker
	CanSelfUpdate  bool   `json:"can_self_update"` // 是否支持一键更新
	DockerSock     bool   `json:"docker_sock"`     // 容器环境是否挂载了 docker.sock
}

// UpdateProgress 更新执行状态与进度
type UpdateProgress struct {
	Status   string `json:"status"`   // idle, checking, downloading, applying, restarting, success, failed
	Progress int    `json:"progress"` // 0-100
	Message  string `json:"message"`
	Error    string `json:"error,omitempty"`
}

type Manager struct {
	currentVersion string
	healthURL      string
	client         *http.Client
	mu             sync.RWMutex
	progress       UpdateProgress
	lastCheck      time.Time
	cachedInfo     *UpdateInfo
}

var DefaultManager *Manager

// 发行构建使用官方渠道。原生集成测试在链接时改用本地的测试发行包与校验清单。
var releaseMetadataURL = "https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest"
var releaseDownloadBase = "https://github.com/xiaoyueRX/Ani-Go/releases/download"

func InitManager(currentVersion string) *Manager {
	DefaultManager = &Manager{
		currentVersion: currentVersion,
		healthURL:      "http://127.0.0.1:20001/api/health",
		client:         httpx.New(30 * time.Second),
		progress: UpdateProgress{
			Status:  "idle",
			Message: "准备就绪",
		},
	}
	return DefaultManager
}

// SetListenAddress 指定更新后需要验证的本实例监听地址。
func (m *Manager) SetListenAddress(host string, port int) {
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	m.healthURL = "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/api/health"
}

// DetectEnvironment 检测当前运行环境
func DetectEnvironment() (EnvironmentType, bool) {
	isDocker := false
	if _, err := os.Stat("/.dockerenv"); err == nil {
		isDocker = true
	} else if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") || strings.Contains(string(data), "containerd") {
			isDocker = true
		}
	}

	if isDocker {
		hasSock := false
		if _, err := os.Stat("/var/run/docker.sock"); err == nil {
			hasSock = true
		}
		return EnvDocker, hasSock
	}
	return EnvBinary, false
}

// CheckUpdate 检测云端 Release
func (m *Manager) CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	env, hasSock := DetectEnvironment()
	canSelfUpdate := env == EnvBinary

	info := &UpdateInfo{
		CurrentVersion: m.currentVersion,
		Environment:    string(env),
		CanSelfUpdate:  canSelfUpdate,
		DockerSock:     hasSock,
	}

	endpoints := []string{
		releaseMetadataURL,
		"https://ghproxy.net/" + releaseMetadataURL,
		"https://ghfast.top/" + releaseMetadataURL,
	}

	type release struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
	}

	var releaseData release
	var lastErr error
	for _, ep := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Ani-Go-Updater/1.0")
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := m.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			var candidate release
			decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&candidate)
			resp.Body.Close()
			if decodeErr == nil && candidate.TagName != "" {
				releaseData = candidate
				lastErr = nil
				break
			}
			lastErr = decodeErr
		} else {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}

	if releaseData.TagName == "" {
		if lastErr == nil {
			lastErr = fmt.Errorf("发布响应未包含版本号")
		}
		return info, fmt.Errorf("检查云端更新失败: %w", lastErr)
	}

	info.LatestVersion = releaseData.TagName
	info.ReleaseNotes = releaseData.Body
	info.PublishedAt = releaseData.PublishedAt

	current := "v" + strings.TrimPrefix(m.currentVersion, "v")
	latest := "v" + strings.TrimPrefix(releaseData.TagName, "v")
	info.HasUpdate = semver.IsValid(current) && semver.IsValid(latest) && semver.Compare(latest, current) > 0

	m.cachedInfo = info
	m.lastCheck = time.Now()

	if info.HasUpdate {
		log.Printf("🔔 [更新检查] 发现云端新版本: %s (当前: %s)", info.LatestVersion, info.CurrentVersion)
	}
	return info, nil
}

// GetProgress 获取当前更新进度
func (m *Manager) GetProgress() UpdateProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.progress
}

// ExecuteUpdate 异步执行一键平滑更新
func (m *Manager) ExecuteUpdate(ctx context.Context) error {
	if env, _ := DetectEnvironment(); env == EnvDocker {
		return fmt.Errorf("请在部署目录执行 docker compose pull ani-go 和 docker compose up -d ani-go 以重建容器")
	}
	m.mu.Lock()
	if m.progress.Status == "downloading" || m.progress.Status == "applying" || m.progress.Status == "restarting" {
		m.mu.Unlock()
		return fmt.Errorf("更新任务正在进行中，请勿重复触发")
	}
	m.progress = UpdateProgress{
		Status:   "downloading",
		Progress: 10,
		Message:  "正在连接云端更新服务...",
	}
	m.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ [更新管理器] 异常崩溃恢复: %v", r)
				m.mu.Lock()
				m.progress = UpdateProgress{
					Status:  "failed",
					Message: fmt.Sprintf("更新异常中断: %v", r),
					Error:   fmt.Sprintf("%v", r),
				}
				m.mu.Unlock()
			}
		}()

		var err error
		info, checkErr := m.CheckUpdate(context.Background())
		if checkErr != nil {
			err = checkErr
		} else if !info.HasUpdate {
			err = fmt.Errorf("没有高于当前版本的可用更新")
		} else {
			err = m.updateBinary(context.Background(), info.LatestVersion)
		}

		m.mu.Lock()
		if err != nil {
			log.Printf("❌ [更新管理器] 更新失败: %v", err)
			m.progress = UpdateProgress{
				Status:  "failed",
				Message: "更新失败: " + err.Error(),
				Error:   err.Error(),
			}
		}
		m.mu.Unlock()
	}()

	return nil
}

// updateBinary 独立二进制自更新与原地替换
func (m *Manager) updateBinary(ctx context.Context, targetVersion string) error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	archName := goarch
	if goarch == "arm" {
		archName = "armv7"
	}

	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	pkgName := fmt.Sprintf("ani-go-%s-%s-%s.%s", targetVersion, goos, archName, ext)
	downloadURL := fmt.Sprintf("%s/%s/%s", releaseDownloadBase, targetVersion, pkgName)

	log.Printf("📦 [自更新] 正在下载系统对应发行包: %s", pkgName)
	m.mu.Lock()
	m.progress = UpdateProgress{Status: "downloading", Progress: 30, Message: "正在下载发布包: " + pkgName}
	m.mu.Unlock()

	downloadCandidates := []string{
		downloadURL,
		"https://ghproxy.net/" + downloadURL,
		"https://ghfast.top/" + downloadURL,
	}

	const archiveLimit = 160 << 20
	// 摘要只从官方发布渠道读取，镜像下载的包使用同一份独立摘要验证。
	manifest, err := m.fetchRelease(ctx, strings.TrimSuffix(downloadURL, pkgName)+"checksums.txt", 1<<20)
	if err != nil {
		return fmt.Errorf("获取官方校验清单失败: %w", err)
	}
	var program []byte
	binName := "anigo"
	if goos == "windows" {
		binName += ".exe"
	}
	for _, u := range downloadCandidates {
		var archive []byte
		archive, err = m.fetchRelease(ctx, u, archiveLimit)
		if err != nil {
			continue
		}
		if err = verifyPackage(archive, manifest, pkgName); err != nil {
			continue
		}
		program, err = extractProgram(archive, ext, binName)
		break
	}
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.progress = UpdateProgress{Status: "applying", Progress: 75, Message: "发行包校验成功，正在准备交接"}
	m.mu.Unlock()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	pending, err := startHandoff(executable, program, targetVersion, m.healthURL)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.progress = UpdateProgress{Status: "restarting", Progress: 90, Message: "等待新版实例健康检查"}
	m.mu.Unlock()
	if pending != nil {
		return <-pending
	}
	return nil
}

func (m *Manager) fetchRelease(ctx context.Context, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Ani-Go-Updater")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("发行文件返回 HTTP %d", resp.StatusCode)
	}
	return readLimited(resp.Body, limit)
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("发行文件超过大小限制")
	}
	return b, nil
}

func verifyPackage(data, manifest []byte, name string) error {
	sum := sha256.Sum256(data)
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && strings.EqualFold(fields[0], hex.EncodeToString(sum[:])) {
			return nil
		}
	}
	return fmt.Errorf("发行包 %s 的 SHA-256 校验失败", name)
}

func extractProgram(data []byte, ext, name string) ([]byte, error) {
	const maxProgram = 256 << 20
	if ext == "zip" {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range z.File {
			if path.Base(f.Name) != name || !f.Mode().IsRegular() {
				continue
			}
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := readLimited(r, maxProgram)
			r.Close()
			if err != nil {
				return nil, err
			}
			if len(b) == 0 || uint64(len(b)) != f.UncompressedSize64 {
				return nil, fmt.Errorf("ZIP 中程序长度有误")
			}
			return b, nil
		}
	} else {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		// 读取至真正的 gzip EOF，以验证压缩包尾部校验码，并限制总解压大小。
		limited := &io.LimitedReader{R: gr, N: maxProgram + 1}
		tr := tar.NewReader(limited)
		var program []byte
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if path.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
				if program != nil {
					return nil, fmt.Errorf("归档包含重复的程序文件")
				}
				program, err = readLimited(tr, maxProgram)
				if err != nil {
					return nil, err
				}
				if int64(len(program)) != h.Size {
					return nil, io.ErrUnexpectedEOF
				}
			}
		}
		if _, err := io.Copy(io.Discard, limited); err != nil {
			return nil, err
		}
		if limited.N <= 0 {
			return nil, fmt.Errorf("解压包超过大小限制")
		}
		if len(program) > 0 {
			return program, nil
		}
	}
	return nil, fmt.Errorf("归档中缺少可执行文件 %s", name)
}
