package downloader

import "math"

// 下载器百分比降为界面展示精度时，尚有剩余字节的任务最多显示为最接近 1 的较小值。
func downloadProgress(fraction float64, remaining *int64) float32 {
	if remaining != nil && *remaining == 0 {
		return 1
	}
	progress := float32(fraction)
	if progress >= 1 && (fraction < 1 || remaining != nil && *remaining > 0) {
		return math.Nextafter32(1, 0)
	}
	return progress
}
