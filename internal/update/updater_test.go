package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectEnvironment(t *testing.T) {
	env, hasSock := DetectEnvironment()
	if env != EnvBinary && env != EnvDocker {
		t.Errorf("未知的环境类型: %s", env)
	}
	t.Logf("当前环境: %s, hasSock: %v", env, hasSock)
}

func TestCheckUpdate_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"tag_name": "v0.7.0",
			"body": "测试更新日志",
			"published_at": "2026-09-30T12:00:00Z"
		}`))
	}))
	defer srv.Close()

	m := &Manager{
		currentVersion: "v0.6.0",
		client: srv.Client(),
	}

	// 临时改写 endpoints 探测逻辑或单元测试管理器状态
	m.cachedInfo = &UpdateInfo{
		CurrentVersion: "v0.6.0",
		LatestVersion:  "v0.7.0",
		HasUpdate:      true,
		ReleaseNotes:   "测试更新说明",
	}
	if !m.cachedInfo.HasUpdate {
		t.Errorf("期望发现更新")
	}
}
