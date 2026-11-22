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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
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
	client         *http.Client
	mu             sync.RWMutex
	progress       UpdateProgress
}

var DefaultManager *Manager

func InitManager(currentVersion string) *Manager {
	DefaultManager = &Manager{
		currentVersion: currentVersion,
		client:         httpx.New(30 * time.Second),
		progress: UpdateProgress{
			Status:  "idle",
			Message: "准备就绪",
		},
	}
	return DefaultManager
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

type releaseInfo struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"assets"`
}

// 发布版本及其校验值必须由官方发布接口提供。
func (m *Manager) latestRelease(ctx context.Context) (releaseInfo, error) {
	var release releaseInfo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest", nil)
	if err != nil {
		return release, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return release, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release, fmt.Errorf("读取官方发布信息失败: HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return release, err
	}
	if release.TagName == "" {
		return release, fmt.Errorf("发布信息缺少版本号")
	}
	return release, nil
}

func (m *Manager) CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	env, hasSock := DetectEnvironment()
	info := &UpdateInfo{CurrentVersion: m.currentVersion, Environment: string(env), CanSelfUpdate: env == EnvBinary, DockerSock: hasSock}
	release, err := m.latestRelease(ctx)
	if err != nil {
		return info, err
	}
	info.LatestVersion = release.TagName
	info.PublishedAt = release.PublishedAt
	info.ReleaseNotes = release.Body
	info.HasUpdate = strings.TrimPrefix(release.TagName, "v") != strings.TrimPrefix(m.currentVersion, "v")
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
		return fmt.Errorf("请通过部署工具拉取镜像并重建容器；Docker Compose 可执行 docker compose pull 和 docker compose up -d")
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

		err := m.updateBinary(context.Background())

		m.mu.Lock()
		if err != nil {
			log.Printf("❌ [更新管理器] 更新失败: %v", err)
			m.progress = UpdateProgress{
				Status:  "failed",
				Message: "更新失败: " + err.Error(),
				Error:   err.Error(),
			}
		} else {
			m.progress = UpdateProgress{
				Status:   "success",
				Progress: 100,
				Message:  "更新已完成，服务正在重启...",
			}
		}
		m.mu.Unlock()
	}()

	return nil
}

// updateBinary 独立二进制自更新与原地替换
func (m *Manager) updateBinary(ctx context.Context) error {
	release, err := m.latestRelease(ctx)
	if err != nil {
		return err
	}
	targetVersion := release.TagName
	if targetVersion == m.currentVersion {
		return fmt.Errorf("已经是当前发布版本")
	}

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
	var expectedDigest string
	var archiveSize int64
	for _, asset := range release.Assets {
		if asset.Name == pkgName {
			expectedDigest = asset.Digest
			archiveSize = asset.Size
			break
		}
	}
	if !strings.HasPrefix(expectedDigest, "sha256:") || len(expectedDigest) != len("sha256:")+sha256.Size*2 {
		return fmt.Errorf("官方发布信息缺少 %s 的 SHA-256 校验值", pkgName)
	}
	if archiveSize <= 0 {
		return fmt.Errorf("官方发布信息缺少归档大小")
	}
	downloadURL := fmt.Sprintf("https://github.com/xiaoyueRX/Ani-Go/releases/download/%s/%s", targetVersion, pkgName)

	log.Printf("📦 [自更新] 正在下载系统对应发行包: %s", pkgName)
	m.mu.Lock()
	m.progress = UpdateProgress{Status: "downloading", Progress: 30, Message: "正在下载发布包: " + pkgName}
	m.mu.Unlock()

	downloadCandidates := []string{
		downloadURL,
		"https://ghproxy.net/" + downloadURL,
		"https://ghfast.top/" + downloadURL,
	}

	var resp *http.Response
	var fetchErr error
	for _, u := range downloadCandidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Ani-Go-Updater/1.0")
		res, err := m.client.Do(req)
		if err == nil && res.StatusCode == http.StatusOK {
			resp = res
			fetchErr = nil
			break
		}
		if res != nil {
			res.Body.Close()
		}
		fetchErr = fmt.Errorf("下载失败")
	}

	if fetchErr != nil || resp == nil {
		return fmt.Errorf("无法从任何候选源下载发行包: %v", fetchErr)
	}
	defer resp.Body.Close()

	bodyData, err := io.ReadAll(io.LimitReader(resp.Body, archiveSize+1))
	if err != nil {
		return fmt.Errorf("读取发布包失败: %w", err)
	}

	m.mu.Lock()
	m.progress = UpdateProgress{Status: "applying", Progress: 70, Message: "正在解压并校验可执行文件..."}
	m.mu.Unlock()

	if int64(len(bodyData)) != archiveSize {
		return fmt.Errorf("发布包大小校验失败")
	}
	digest := sha256.Sum256(bodyData)
	if expectedDigest != "sha256:"+hex.EncodeToString(digest[:]) {
		return fmt.Errorf("发布包 SHA-256 校验失败")
	}
	binaryBytes, err := unpackBinary(bodyData, ext)
	if err != nil {
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return err
	}
	if err := installBinary(execPath, binaryBytes); err != nil {
		return err
	}

	m.mu.Lock()
	m.progress = UpdateProgress{Status: "restarting", Progress: 90, Message: "文件更新完毕，正在自愈重启系统..."}
	m.mu.Unlock()

	log.Printf("🚀 [自更新] 二进制文件原子替换成功: %s -> 即将重启自身", execPath)

	// 更新任务本身已在后台运行；启动失败必须反馈给调用方。
	time.Sleep(time.Second)
	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		if rollbackErr := os.Rename(execPath+".old", execPath); rollbackErr != nil {
			return fmt.Errorf("启动新版失败: %v；恢复旧版失败: %w", err, rollbackErr)
		}
		return fmt.Errorf("启动新版失败，已恢复原程序: %w", err)
	}
	os.Exit(0)
	return nil
}

func unpackBinary(bodyData []byte, ext string) ([]byte, error) {
	var binaryBytes []byte
	binTargetName := "anigo"
	if ext == "zip" {
		binTargetName = "anigo.exe"
	}

	if ext == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(bodyData), int64(len(bodyData)))
		if err != nil {
			return nil, fmt.Errorf("解析 ZIP 失败: %w", err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == binTargetName {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				binaryBytes, err = io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return nil, fmt.Errorf("读取 ZIP 程序失败: %w", err)
				}
				break
			}
		}
	} else {
		gr, err := gzip.NewReader(bytes.NewReader(bodyData))
		if err != nil {
			return nil, fmt.Errorf("解析 GZIP 失败: %w", err)
		}
		defer gr.Close()
		tr := tar.NewReader(gr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if filepath.Base(hdr.Name) == binTargetName {
				binaryBytes, err = io.ReadAll(tr)
				if err != nil {
					return nil, fmt.Errorf("读取归档程序失败: %w", err)
				}
				break
			}
		}
		if _, err := io.Copy(io.Discard, gr); err != nil {
			return nil, fmt.Errorf("GZIP 完整性检查失败: %w", err)
		}
	}

	if len(binaryBytes) == 0 {
		return nil, fmt.Errorf("在发行包中未找到可执行文件 %s", binTargetName)
	}

	return binaryBytes, nil
}

// installBinary 在同目录准备并落盘新程序，保留上一个可执行版本。
func installBinary(path string, data []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(path), ".anigo-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if _, err = staged.Write(data); err == nil {
		err = staged.Chmod(0755)
	}
	if err == nil {
		err = staged.Sync()
	}
	closeErr := staged.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	old := path + ".old"
	if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := os.Rename(path, old); err != nil {
			return err
		}
		if err := os.Rename(staged.Name(), path); err != nil {
			if restoreErr := os.Rename(old, path); restoreErr != nil {
				return fmt.Errorf("替换失败: %v；回滚失败: %w", err, restoreErr)
			}
			return err
		}
		return nil
	}
	if err := os.Link(path, old); err != nil {
		return fmt.Errorf("保留旧版本失败: %w", err)
	}
	return os.Rename(staged.Name(), path)
}
