package database

import (
	"path/filepath"
	"testing"
)

func TestBootstrapNewVolumeAndCredentials(t *testing.T) {
	old := DB
	t.Cleanup(func() { DB = old })
	for _, preset := range []string{"", "configured-unique-password"} {
		t.Setenv("ANIGO_ADMIN_PASSWORD", preset)
		if err := Init(filepath.Join(t.TempDir(), "db", "anigo.db")); err != nil {
			t.Fatalf("初始化空卷: %v", err)
		}
		sqlDB, _ := DB.DB()
		t.Cleanup(func() { sqlDB.Close() })
		var seen []string
		hash := func(p string) (string, error) { seen = append(seen, p); return "hashed", nil }
		if err := InitDefaultUser(hash); err != nil {
			t.Fatal(err)
		}
		if err := InitDefaultUser(hash); err != nil {
			t.Fatal(err)
		}
		if len(seen) != 1 || (preset == "" && len(seen[0]) < 20) || (preset != "" && seen[0] != preset) {
			t.Fatal("初始化口令或重启行为错误")
		}
	}
}
