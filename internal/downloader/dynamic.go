package downloader

import (
	"context"
	"fmt"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// DynamicDownloader 是一个线程安全的动态代理下载器
// 允许在运行时通过 Swap() 无缝切换底层下载器实现（qBittorrent / Transmission / Aria2），无需重启服务
type DynamicDownloader struct {
	mu      sync.RWMutex
	current core.Downloader
}

// NewDynamicDownloader 创建动态代理下载器
func NewDynamicDownloader(initial core.Downloader) *DynamicDownloader {
	return &DynamicDownloader{
		current: initial,
	}
}

// Swap 原子性替换当前生效的底层下载器
func (d *DynamicDownloader) Swap(newDL core.Downloader) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.current = newDL
}

// GetCurrent 获取当前底层的下载器实例
func (d *DynamicDownloader) GetCurrent() core.Downloader {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current
}

func (d *DynamicDownloader) Name() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return "none"
	}
	return d.current.Name()
}

func (d *DynamicDownloader) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	// 磁盘空间熔断保护：在向下载器提交任务前，首先检查目标路径磁盘空间
	if err := CheckDiskSpaceGuard(savePath); err != nil {
		return err
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return fmt.Errorf("下载器未就绪")
	}
	return d.current.Add(ctx, item, savePath)
}

func (d *DynamicDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return nil, fmt.Errorf("下载器未就绪")
	}
	return d.current.List(ctx)
}

func (d *DynamicDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return core.DownloadTask{}, fmt.Errorf("下载器未就绪")
	}
	return d.current.GetStatus(ctx, hash)
}

func (d *DynamicDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return fmt.Errorf("下载器未就绪")
	}
	return d.current.Delete(ctx, hash, deleteFiles)
}

func (d *DynamicDownloader) Pause(ctx context.Context, hash string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return fmt.Errorf("下载器未就绪")
	}
	return d.current.Pause(ctx, hash)
}

func (d *DynamicDownloader) Resume(ctx context.Context, hash string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return fmt.Errorf("下载器未就绪")
	}
	return d.current.Resume(ctx, hash)
}

func (d *DynamicDownloader) IsAvailable(ctx context.Context) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.current == nil {
		return false
	}
	return d.current.IsAvailable(ctx)
}
