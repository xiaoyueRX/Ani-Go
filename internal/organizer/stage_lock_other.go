//go:build !linux

package organizer

import "os"

// 无可用目录锁的平台始终隔离各自的暂存目录。
func lockStageDirectory(*os.File) bool { return false }
