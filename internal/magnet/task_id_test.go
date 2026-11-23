package magnet

import (
	"strings"
	"testing"
)

func TestCanonicalTaskIDFoldsOnlyHexInfoHashes(t *testing.T) {
	for _, hash := range []string{strings.Repeat("aB", 20), strings.Repeat("cD", 32)} {
		if got := CanonicalTaskID(hash); got != strings.ToLower(hash) {
			t.Errorf("哈希未规范化：%q", got)
		}
	}
	for _, id := range []string{"Task-A", "FEDCBA9876543210", strings.Repeat("Ax", 20), strings.Repeat("AZ", 16)} {
		if got := CanonicalTaskID(id); got != id {
			t.Errorf("不透明任务标识 %q 变成了 %q", id, got)
		}
	}
}
