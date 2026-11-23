package source

import (
	"context"
	"net"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestSearchReturnsErrorWhenAllMirrorsFail(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := listener.Addr().String()
	listener.Close()
	m := NewMikanSource(host, "", nil)
	if _, err := m.SearchAnime(context.Background(), "unreachable-mirror-test"); err == nil {
		t.Fatal("所有镜像连接失败时应返回错误")
	}
}

func TestHistoryUsesSeriesTitles(t *testing.T) {
	var queries []string
	items, err := searchHistoryTitles(context.Background(), []string{"", "系列名", "系列名", "Series Title"}, func(_ context.Context, title string) ([]core.TorrentItem, error) {
		queries = append(queries, title)
		if title == "Series Title" {
			return []core.TorrentItem{{Title: title}}, nil
		}
		return nil, nil
	})
	if err != nil || len(items) != 1 || len(queries) != 2 || queries[0] != "系列名" || queries[1] != "Series Title" {
		t.Fatalf("查询 %+v，结果 %+v，错误 %v", queries, items, err)
	}
}
