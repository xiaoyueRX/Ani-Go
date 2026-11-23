package source

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestMultiSourceSettingsUseDeploymentFallback(t *testing.T) {
	previous := database.DB
	if err := database.Init(filepath.Join(t.TempDir(), "settings.db")); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB.Close()
		database.DB = previous
	})
	for _, prefix := range []string{"NYAA", "ACGRIP", "ANIMETOSHO"} {
		t.Setenv(prefix+"_DOMAIN", "")
		t.Setenv(prefix+"_ENABLED", "")
	}
	ms := NewMultiSource(NewNyaaSource("deployment.example"), NewACGRIPSource(""))
	put := func(key, value string) {
		t.Helper()
		if err := database.DB.Save(&database.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	check := func(want map[string]string) {
		t.Helper()
		got := make(map[string]string)
		for _, src := range ms.Sources() {
			switch src := src.(type) {
			case *NyaaSource:
				got[src.Name()] = src.domain
			case *ACGRIPSource:
				got[src.Name()] = src.domain
			case *AnimeToshoSource:
				got[src.Name()] = src.domain
			default:
				t.Fatalf("未知搜索源 %T", src)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("可用来源 %v，期望 %v", got, want)
		}
	}
	check(map[string]string{"Nyaa": "deployment.example", "ACG.RIP": "acg.rip"})
	put("NYAA_DOMAIN", "db.example")
	put("ACGRIP_ENABLED", "false")
	put("ANIMETOSHO_ENABLED", "1")
	check(map[string]string{"Nyaa": "db.example", "AnimeTosho": "feed.animetosho.org"})
	put("NYAA_DOMAIN", "")
	check(map[string]string{"Nyaa": "deployment.example", "AnimeTosho": "feed.animetosho.org"})
	put("NYAA_DOMAIN", "db.example")
	t.Setenv("NYAA_DOMAIN", "environment.example")
	check(map[string]string{"Nyaa": "environment.example", "AnimeTosho": "feed.animetosho.org"})
	put("ANIMETOSHO_ENABLED", "false")
	put("NYAA_ENABLED", "false")
	check(map[string]string{})
	put("ANIMETOSHO_ENABLED", "true")
	t.Setenv("ANIMETOSHO_ENABLED", "false")
	check(map[string]string{})
	t.Setenv("NYAA_ENABLED", "true")
	check(map[string]string{"Nyaa": "environment.example"})
	t.Setenv("NYAA_ENABLED", "")
	if err := database.DB.Where("key = ?", "NYAA_ENABLED").Delete(&database.Setting{}).Error; err != nil {
		t.Fatal(err)
	}
	check(map[string]string{"Nyaa": "environment.example"})
}
