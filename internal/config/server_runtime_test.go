package config

import (
	"slices"
	"testing"
)

func TestServerEnvironmentAndPositiveIntervals(t *testing.T) {
	t.Setenv("HOST", "::1")
	t.Setenv("LOG_PATH", "custom.log")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://one.test, https://two.test")
	t.Setenv("RSS_INTERVAL", "0s")
	c := Load()
	if c.Server.Host != "::1" || c.Server.LogPath != "custom.log" || !slices.Equal(c.Server.AllowedOrigins, []string{"https://one.test", "https://two.test"}) {
		t.Errorf("服务配置未生效: %+v", c.Server)
	}
	if c.Scheduler.RSSInterval != defaults().Scheduler.RSSInterval {
		t.Errorf("无效 RSS 周期 %s", c.Scheduler.RSSInterval)
	}
	c.MergeFromSettings(func(k string) (string, bool) { return "-10s", k == "ORGANIZER_INTERVAL" })
	if c.Scheduler.OrganizerInterval != defaults().Scheduler.OrganizerInterval {
		t.Errorf("无效整理周期 %s", c.Scheduler.OrganizerInterval)
	}
}
