package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// mockStressDownloader 用于并发测试的 Dummy 下载器
type mockStressDownloader struct {
	name string
	mu   sync.Mutex
	cnt  int
}

func (m *mockStressDownloader) Name() string { return m.name }
func (m *mockStressDownloader) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	m.mu.Lock()
	m.cnt++
	m.mu.Unlock()
	return nil
}
func (m *mockStressDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return []core.DownloadTask{
		{Hash: "40charinfohashabcdef1234567890abcdef1234", Name: "MockTask", Status: "downloading"},
	}, nil
}
func (m *mockStressDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return core.DownloadTask{Hash: hash, Name: "MockTask", Status: "downloading"}, nil
}
func (m *mockStressDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	return nil
}
func (m *mockStressDownloader) Pause(ctx context.Context, hash string) error  { return nil }
func (m *mockStressDownloader) Resume(ctx context.Context, hash string) error { return nil }
func (m *mockStressDownloader) IsAvailable(ctx context.Context) bool          { return true }

// TestDynamicDownloader_ConcurrentSwapAndOperations 测试动态代理下载器高并发热切换与并发调用
func TestDynamicDownloader_ConcurrentSwapAndOperations(t *testing.T) {
	dl1 := &mockStressDownloader{name: "dl-1"}
	dl2 := &mockStressDownloader{name: "dl-2"}
	dl3 := &mockStressDownloader{name: "dl-3"}

	dyn := NewDynamicDownloader(dl1)

	const numWorkers = 40
	const numOps = 25

	var wg sync.WaitGroup
	wg.Add(numWorkers + 2)

	// 专门两个 Goroutine 不断并发 Swap
	stopSwap := make(chan struct{})
	go func() {
		defer wg.Done()
		dls := []core.Downloader{dl1, dl2, dl3}
		idx := 0
		for {
			select {
			case <-stopSwap:
				return
			default:
				dyn.Swap(dls[idx%len(dls)])
				idx++
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
	go func() {
		defer wg.Done()
		dls := []core.Downloader{dl3, dl2, dl1}
		idx := 0
		for {
			select {
			case <-stopSwap:
				return
			default:
				dyn.Swap(dls[idx%len(dls)])
				idx++
				time.Sleep(3 * time.Millisecond)
			}
		}
	}()

	// 业务并发调用
	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < numOps; i++ {
				_ = dyn.Name()
				_ = dyn.IsAvailable(ctx)
				_, _ = dyn.List(ctx)
				_, _ = dyn.GetStatus(ctx, "somehash")
				_ = dyn.Pause(ctx, "somehash")
				_ = dyn.Resume(ctx, "somehash")
				_ = dyn.Delete(ctx, "somehash", false)
				_ = dyn.Add(ctx, core.TorrentItem{Title: fmt.Sprintf("t-%d-%d", workerID, i)}, "")
			}
		}()
	}

	for w := 0; w < numWorkers; w++ {
		// wait workers first
	}

	// 等待业务完成
	// 创建一个独立 WaitGroup 只等待 workers
	// 简化方案：直接等待所有 workers 并在 500ms 后关闭 stopSwap
	time.Sleep(50 * time.Millisecond)
	close(stopSwap)
	wg.Wait()
}

// TestTransmission_RetryAndSessionRotationUnderStress 模拟高并发下 Transmission 409 会话更新与网络错误重试
func TestTransmission_RetryAndSessionRotationUnderStress(t *testing.T) {
	var currentSID atomic.Pointer[string]
	initSID := "session-token-v1"
	currentSID.Store(&initSID)

	var conflictCount atomic.Int64
	var successCount atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqSID := r.Header.Get("X-Transmission-Session-Id")
		validSID := *currentSID.Load()

		if reqSID != validSID {
			conflictCount.Add(1)
			// 返回 409 并给新 session id
			newSID := fmt.Sprintf("session-token-v%d", conflictCount.Load()+1)
			currentSID.Store(&newSID)
			w.Header().Set("X-Transmission-Session-Id", newSID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		// 模拟正常响应
		var req trRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		successCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if req.Method == "session-get" {
			_ = json.NewEncoder(w).Encode(trResponse{
				Result:    "success",
				Tag:       req.Tag,
				Arguments: json.RawMessage(`{"version": "4.0.5"}`),
			})
		} else if req.Method == "torrent-get" {
			_ = json.NewEncoder(w).Encode(trResponse{
				Result:    "success",
				Tag:       req.Tag,
				Arguments: json.RawMessage(`{"torrents":[{"id":1,"hashString":"ABCDEF123456","name":"Test Anime","status":4,"percentDone":0.5}]}`),
			})
		} else {
			_ = json.NewEncoder(w).Encode(trResponse{
				Result: "success",
				Tag:    req.Tag,
			})
		}
	}))
	defer server.Close()

	tr := NewTransmission(server.URL, "user", "pass")

	// 1. 测试 IsAvailable 在 session-id 409 轮转下能否自愈
	if !tr.IsAvailable(context.Background()) {
		t.Errorf("Transmission IsAvailable 应该成功自愈 409 会话")
	}

	// 2. 测试 GetStatus 大小写不敏感匹配
	task, err := tr.GetStatus(context.Background(), "abcdef123456") // 小写查询大写 Hash
	if err != nil {
		t.Fatalf("GetStatus 大小写不敏感匹配失败: %v", err)
	}
	if task.Name != "Test Anime" {
		t.Errorf("GetStatus 结果不符: %+v", task)
	}

	// 3. 高并发调用检验锁与重试
	const numConcurrent = 20
	var wg sync.WaitGroup
	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func() {
			defer wg.Done()
			_, _ = tr.List(context.Background())
			_ = tr.Pause(context.Background(), "ABCDEF123456")
			_ = tr.Resume(context.Background(), "ABCDEF123456")
		}()
	}
	wg.Wait()

	if successCount.Load() == 0 {
		t.Errorf("期望至少有成功请求，实际为 0")
	}
}

// TestAria2_ToleranceAndGIDResolution 模拟 Aria2 容错与 40 位 InfoHash 自动转换为 GID
func TestAria2_ToleranceAndGIDResolution(t *testing.T) {
	infoHash := "e4d3c2b1a0e4d3c2b1a0e4d3c2b1a0e4d3c2b1a0"
	gid := "2089b05ecca3d829"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch req.Method {
		case "aria2.getVersion":
			_ = json.NewEncoder(w).Encode(aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"version": "1.36.0"}`),
			})
		case "aria2.tellActive":
			tasks := fmt.Sprintf(`[{"gid":"%s","status":"active","totalLength":"1000","completedLength":"500","bittorrent":{"infoHash":"%s"}}]`, gid, infoHash)
			_ = json.NewEncoder(w).Encode(aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(tasks),
			})
		case "aria2.tellWaiting", "aria2.tellStopped":
			_ = json.NewEncoder(w).Encode(aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`[]`),
			})
		case "aria2.tellStatus":
			_ = json.NewEncoder(w).Encode(aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(fmt.Sprintf(`{"gid":"%s","status":"active","totalLength":"1000","completedLength":"500","bittorrent":{"infoHash":"%s"}}`, gid, infoHash)),
			})
		case "aria2.pause", "aria2.unpause", "aria2.remove":
			_ = json.NewEncoder(w).Encode(aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(fmt.Sprintf(`"%s"`, gid)),
			})
		default:
			_ = json.NewEncoder(w).Encode(aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`"OK"`),
			})
		}
	}))
	defer server.Close()

	ar := NewAria2(server.URL, "secret123")

	// 1. IsAvailable
	if !ar.IsAvailable(context.Background()) {
		t.Errorf("Aria2 IsAvailable 应返回 true")
	}

	// 2. 使用 40 位 InfoHash 查询 GetStatus (应自动解析并找到)
	task, err := ar.GetStatus(context.Background(), strings.ToUpper(infoHash))
	if err != nil {
		t.Fatalf("通过 InfoHash 获取 Aria2 状态失败: %v", err)
	}
	if task.Hash != infoHash {
		t.Errorf("Hash 匹配不符: 期望 %s, 得到 %s", infoHash, task.Hash)
	}

	// 3. 使用 40 位 InfoHash 执行 Pause / Resume / Delete (验证 resolveGID)
	if err := ar.Pause(context.Background(), infoHash); err != nil {
		t.Errorf("通过 InfoHash 暂停失败: %v", err)
	}
	if err := ar.Resume(context.Background(), infoHash); err != nil {
		t.Errorf("通过 InfoHash 恢复失败: %v", err)
	}
	if err := ar.Delete(context.Background(), infoHash, false); err != nil {
		t.Errorf("通过 InfoHash 删除失败: %v", err)
	}
}

// TestDownloader_URLNormalization 验证 Aria2 与 Transmission 配置 URL 后缀自动清洗
func TestDownloader_URLNormalization(t *testing.T) {
	ar1 := NewAria2("http://localhost:6800/jsonrpc", "token")
	if ar1.host != "http://localhost:6800" {
		t.Errorf("Aria2 host 清洗失败: 期望 http://localhost:6800, 得到 %s", ar1.host)
	}
	ar2 := NewAria2("http://localhost:6800/jsonrpc/", "token")
	if ar2.host != "http://localhost:6800" {
		t.Errorf("Aria2 host 清洗失败: 期望 http://localhost:6800, 得到 %s", ar2.host)
	}
	ar3 := NewAria2("http://localhost:6800", "token")
	if ar3.host != "http://localhost:6800" {
		t.Errorf("Aria2 host 清洗失败: 期望 http://localhost:6800, 得到 %s", ar3.host)
	}

	tr1 := NewTransmission("http://localhost:9091/transmission/rpc", "u", "p")
	if tr1.host != "http://localhost:9091" {
		t.Errorf("Transmission host 清洗失败: 期望 http://localhost:9091, 得到 %s", tr1.host)
	}
	tr2 := NewTransmission("http://localhost:9091/transmission/rpc/", "u", "p")
	if tr2.host != "http://localhost:9091" {
		t.Errorf("Transmission host 清洗失败: 期望 http://localhost:9091, 得到 %s", tr2.host)
	}
	tr3 := NewTransmission("http://localhost:9091/transmission", "u", "p")
	if tr3.host != "http://localhost:9091" {
		t.Errorf("Transmission host 清洗失败: 期望 http://localhost:9091, 得到 %s", tr3.host)
	}
}

// TestAria2_HTTPErrorAndNameExtraction 验证 Aria2 HTTP 错误保护与名称提取
func TestAria2_HTTPErrorAndNameExtraction(t *testing.T) {
	// 1. 测试非 200 HTTP 响应拦截（防 JSON 解析崩溃）
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("<html><body>401 Unauthorized</body></html>"))
	}))
	defer errServer.Close()

	arErr := NewAria2(errServer.URL, "badtoken")
	_, err := arErr.List(context.Background())
	if err == nil {
		t.Fatalf("预期 401 报错，实际未报错")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("错误信息未包含 HTTP 状态码 401: %v", err)
	}

	// 2. 测试从 Files 中提取任务名称与 complete -> completed 状态规范化
	st := aria2Status{
		GID:             "1234567890abcdef",
		Status:          "complete",
		TotalLength:     2048,
		CompletedLength: 2048,
		Dir:             "/downloads",
		Files: []struct {
			Path string `json:"path"`
		}{
			{Path: "/downloads/Sousou_no_Frieren/ep01.mp4"},
		},
	}
	task := convertAria2Status(st)
	if task.Name != "Sousou_no_Frieren" {
		t.Errorf("任务名称提取失败: 期望 'Sousou_no_Frieren', 得到 %q", task.Name)
	}
	if task.Status != "completed" {
		t.Errorf("状态规范化失败: 期望 'completed', 得到 %q", task.Status)
	}
}
