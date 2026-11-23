package downloader

import (
	"encoding/json"
	"testing"
)

func TestAria2HTTPTaskHasUsableGID(t *testing.T) {
	var wire aria2Status
	if err := json.Unmarshal([]byte(`{"gid":"0123456789abcdef","status":"active","dir":"/downloads","files":[{"path":"/downloads/episode.mkv"}]}`), &wire); err != nil {
		t.Fatal(err)
	}
	if task := convertAria2Status(wire); task.Hash != wire.GID || task.Name != "episode.mkv" {
		t.Fatalf("HTTP 任务丢失 GID: %+v", task)
	}
}
