package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLastByteDoesNotFinishDownload(t *testing.T) {
	for _, status := range []string{"active", "paused"} {
		task := convertAria2Status(aria2Status{Status: status, TotalLength: 1 << 30, CompletedLength: (1 << 30) - 1})
		if isTaskReadyForSeedCleanup(task) {
			t.Fatalf("Aria2 %s 仍缺一个字节却可以清理", status)
		}
	}
	tasks, err := parseQBittorrentList(strings.NewReader(`[{"name":"selected","state":"pausedUP","progress":1,"amount_left":1,"size":1073741824,"completed":1073741823}]`))
	if err != nil || len(tasks) != 1 || isTaskReadyForSeedCleanup(tasks[0]) {
		t.Fatalf("qB 尚有剩余字节 %+v %v", tasks, err)
	}
}

func TestTransmissionUsesSelectedRemainingBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[{"status":0,"percentDone":0.999999999,"totalSize":2147483648,"sizeWhenDone":1073741824,"leftUntilDone":1,"downloadedEver":3221225472},{"status":0,"percentDone":1,"totalSize":2147483648,"sizeWhenDone":1073741824,"leftUntilDone":0,"downloadedEver":3221225472}]}}`))
	}))
	defer server.Close()
	tasks, err := NewTransmission(server.URL, "", "").List(context.Background())
	if err != nil || len(tasks) != 2 {
		t.Fatalf("任务列表 %+v %v", tasks, err)
	}
	if isTaskReadyForSeedCleanup(tasks[0]) || !isTaskReadyForSeedCleanup(tasks[1]) || tasks[0].Done != 1073741823 {
		t.Fatalf("选择文件的剩余字节被忽略 %+v", tasks)
	}
}
