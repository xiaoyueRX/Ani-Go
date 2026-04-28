package scheduler

import (
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestBuildFilter_FromSubscription(t *testing.T) {
	sub := database.Subscription{
		SubgroupName: "千夏字幕组",
		FilterJSON:   `{"include_keywords":["1080p","CHS"],"exclude_keywords":["BIG5"],"resolution":"1080p"}`,
	}
	filter := buildFilter(sub)
	if filter.PreferSubgroup != "千夏字幕组" {
		t.Errorf("PreferSubgroup = %q, 期望 %q", filter.PreferSubgroup, "千夏字幕组")
	}
	if len(filter.IncludeKeywords) != 2 {
		t.Errorf("IncludeKeywords 数量 = %d, 期望 2", len(filter.IncludeKeywords))
	}
	if filter.IncludeKeywords[0] != "1080p" {
		t.Errorf("IncludeKeywords[0] = %q, 期望 %q", filter.IncludeKeywords[0], "1080p")
	}
	if filter.Resolution != "1080p" {
		t.Errorf("Resolution = %q, 期望 %q", filter.Resolution, "1080p")
	}
}

func TestBuildFilter_NoFilterJSON(t *testing.T) {
	sub := database.Subscription{
		SubgroupName: "喵萌奶茶屋",
	}
	filter := buildFilter(sub)
	if filter.PreferSubgroup != "喵萌奶茶屋" {
		t.Errorf("PreferSubgroup = %q, 期望 %q", filter.PreferSubgroup, "喵萌奶茶屋")
	}
	if len(filter.IncludeKeywords) != 0 {
		t.Error("无 FilterJSON 时 IncludeKeywords 应为空")
	}
}

func TestBuildFilter_InvalidJSON(t *testing.T) {
	sub := database.Subscription{
		FilterJSON: "not valid json!!!",
	}
	// 不应 panic
	filter := buildFilter(sub)
	if filter.PreferSubgroup != "" {
		t.Error("无 SubgroupName 时 PreferSubgroup 应为空")
	}
}

// 确保 buildFilter 返回的 Filter 可以用在 Mikan Source 上
func TestBuildFilter_ReturnsValidFilter(t *testing.T) {
	sub := database.Subscription{
		SubgroupName: "桜都字幕组",
		FilterJSON:   `{"include_keywords":["1080p"],"exclude_keywords":["720p"]}`,
	}
	filter := buildFilter(sub)
	// 类型检查
	var _ core.Filter = filter
	if filter.PreferSubgroup != "桜都字幕组" {
		t.Errorf("PreferSubgroup 不匹配")
	}
}
