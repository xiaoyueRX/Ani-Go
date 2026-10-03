package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
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
	lastCheck      time.Time
	cachedInfo     *UpdateInfo
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

// CheckUpdate 检测云端 Release
func (m *Manager) CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	env, hasSock := DetectEnvironment()
	canSelfUpdate := (env == EnvBinary) || (env == EnvDocker && hasSock)

	info := &UpdateInfo{
		CurrentVersion: m.currentVersion,
		Environment:    string(env),
		CanSelfUpdate:  canSelfUpdate,
		DockerSock:     hasSock,
	}

	endpoints := []string{
		"https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest",
		"https://ghproxy.net/https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest",
		"https://ghfast.top/https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest",
	}

	var releaseData struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
	}

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
			decodeErr := json.NewDecoder(resp.Body).Decode(&releaseData)
			resp.Body.Close()
			if decodeErr == nil && releaseData.TagName != "" {
				lastErr = nil
				break
			}
		} else {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}

	if lastErr != nil && releaseData.TagName == "" {
		return info, fmt.Errorf("检查云端更新失败: %w", lastErr)
	}

	info.LatestVersion = releaseData.TagName
	info.ReleaseNotes = releaseData.Body
	info.PublishedAt = releaseData.PublishedAt

	currClean := strings.TrimPrefix(m.currentVersion, "v")
	latestClean := strings.TrimPrefix(releaseData.TagName, "v")
	info.HasUpdate = (latestClean != "" && currClean != latestClean)

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

		env, hasSock := DetectEnvironment()
		var err error
		if env == EnvDocker {
			if !hasSock {
				err = fmt.Errorf("Docker 容器未挂载 /var/run/docker.sock，无法自动拉取镜像与自愈")
			} else {
				err = m.updateDocker(context.Background())
			}
		} else {
			err = m.updateBinary(context.Background())
		}

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
	targetVersion := ""
	if m.cachedInfo != nil {
		targetVersion = m.cachedInfo.LatestVersion
	}
	if targetVersion == "" {
		info, err := m.CheckUpdate(ctx)
		if err != nil {
			return err
		}
		targetVersion = info.LatestVersion
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

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取发布包失败: %w", err)
	}

	m.mu.Lock()
	m.progress = UpdateProgress{Status: "applying", Progress: 70, Message: "正在解压并校验可执行文件..."}
	m.mu.Unlock()

	var binaryBytes []byte
	binTargetName := "anigo"
	if goos == "windows" {
		binTargetName = "anigo.exe"
	}

	if ext == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(bodyData), int64(len(bodyData)))
		if err != nil {
			return fmt.Errorf("解析 ZIP 失败: %w", err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == binTargetName {
				rc, err := f.Open()
				if err != nil {
					return err
					}
				binaryBytes, _ = io.ReadAll(rc)
				rc.Close()
				break
			}
		}
	} else {
		gr, err := gzip.NewReader(bytes.NewReader(bodyData))
		if err != nil {
			return fmt.Errorf("解析 GZIP 失败: %w", err)
		}
		defer gr.Close()
		tr := tar.NewReader(gr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if filepath.Base(hdr.Name) == binTargetName {
				binaryBytes, _ = io.ReadAll(tr)
				break
			}
		}
	}

	if len(binaryBytes) == 0 {
		return fmt.Errorf("在发行包中未找到可执行文件 %s", binTargetName)
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}
	execPath, _ = filepath.EvalSymlinks(execPath)
	execPath, _ = filepath.Abs(execPath)

	newPath := execPath + ".new"
	if err := os.WriteFile(newPath, binaryBytes, 0755); err != nil {
		return fmt.Errorf("写入新版本文件失败: %w", err)
	}

	if goos == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		_ = os.Rename(execPath, oldPath)
		if err := os.Rename(newPath, execPath); err != nil {
			_ = os.Rename(oldPath, execPath)
			return fmt.Errorf("Windows 原子替换失败: %w", err)
		}
	} else {
		if err := os.Rename(newPath, execPath); err != nil {
			return fmt.Errorf("Linux 原地替换文件失败: %w", err)
		}
	}

	m.mu.Lock()
	m.progress = UpdateProgress{Status: "restarting", Progress: 90, Message: "文件更新完毕，正在自愈重启系统..."}
	m.mu.Unlock()

	log.Printf("🚀 [自更新] 二进制文件原子替换成功: %s -> 即将重启自身", execPath)

	go func() {
		time.Sleep(1 * time.Second)
		cmd := exec.Command(execPath, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Printf("❌ 重启新进程失败: %v", err)
			return
		}
		log.Println("👋 旧版进程即将正常交接退出")
		os.Exit(0)
	}()

	return nil
}

// updateDocker 针对 Docker 容器通过挂载的 docker.sock 与 Docker Engine 交互
func (m *Manager) updateDocker(ctx context.Context) error {
	m.mu.Lock()
	m.progress = UpdateProgress{Status: "downloading", Progress: 30, Message: "正在通过 Docker Engine 拉取最新镜像..."}
	m.mu.Unlock()

	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 5 * time.Minute,
	}

	imageName := "ghcr.io/xiaoyuerx/ani-go:latest"
	pullURL := fmt.Sprintf("http://localhost/images/create?fromImage=%s", imageName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, nil)
	if err != nil {
		return err
	}
	resp, err := dockerClient.Do(req)
	if err != nil {
		return fmt.Errorf("Docker Socket 连接失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Docker Pull 失败 (%d): %s", resp.StatusCode, string(body))
	}

	_, _ = io.Copy(io.Discard, resp.Body)

	m.mu.Lock()
	m.progress = UpdateProgress{Status: "applying", Progress: 80, Message: "最新镜像拉取完成，准备原地重启容器..."}
	m.mu.Unlock()

	hostname, _ := os.Hostname()
	containerID := strings.TrimSpace(hostname)
	if containerID == "" {
		containerID = "ani-go"
	}

	log.Printf("🐳 [Docker 自更新] 镜像拉取完成，准备重启当前容器 [%s]", containerID)

	go func() {
		time.Sleep(1 * time.Second)
		restartURL := fmt.Sprintf("http://localhost/containers/%s/restart?t=5", containerID)
		rreq, _ := http.NewRequest(http.MethodPost, restartURL, nil)
		_, _ = dockerClient.Do(rreq)
	}()

	return nil
}
