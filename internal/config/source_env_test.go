package config

import "testing"

func TestSourceEnvironmentEnableOverridesDomainDefault(t *testing.T) {
	for _, name := range []string{"NYAA", "ACGRIP", "ANIMETOSHO"} {
		t.Setenv(name+"_DOMAIN", "env.example")
		t.Setenv(name+"_ENABLED", "false")
	}
	c := Load().Sources
	for _, src := range []SourceConfig{c.Nyaa, c.ACGRIP, c.AnimeTosho} {
		if src.Enabled || src.Domain != "env.example" {
			t.Errorf("显式关闭应保留部署域名：%+v", src)
		}
	}
	t.Setenv("NYAA_ENABLED", "")
	t.Setenv("ACGRIP_DOMAIN", "")
	t.Setenv("ACGRIP_ENABLED", "1")
	c = Load().Sources
	if !c.Nyaa.Enabled || !c.ACGRIP.Enabled || c.ACGRIP.Domain != "acg.rip" {
		t.Errorf("仅域名应启用，显式启用允许默认站点：%+v", c)
	}
}
