package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
)

// 每一批只查询已绑定的真实哈希，使 qB 的 GET 地址也适用于常见反向代理。
const presenceBatchSize = 48

func filteredTaskPresence(ids []string, query func([]string) ([]string, error)) (map[string]bool, error) {
	seen := make(map[string]bool)
	var hashes []string
	for _, id := range ids {
		h := strings.ToLower(strings.TrimSpace(id))
		if seen[h] || !magnet.IsHexInfoHash(h) {
			continue
		}
		seen[h] = true
		hashes = append(hashes, h)
	}
	result := make(map[string]bool)
	for batch := range slices.Chunk(hashes, presenceBatchSize) {
		found, err := query(batch)
		if err != nil {
			return nil, err
		}
		for _, h := range batch {
			// 部分客户端用 40 位内部 ID 标识 v2 任务，不能用完整 SHA-256 查空就断定已移除。
			if len(h) == 40 {
				result[h] = false
			}
		}
		for _, raw := range found {
			h := strings.ToLower(raw)
			if !slices.Contains(batch, h) {
				return nil, errors.New("下载器返回的任务标识与定向查询不符")
			}
			result[h] = true
		}
	}
	return result, nil
}

// decodeSingleJSON 拒绝空正文、未闭合的 JSON 或同一响应中多出的第二个值。
func decodeSingleJSON(r io.Reader, out any) error {
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON 正文包含多余数据")
	}
	return nil
}

func (q *QBittorrent) TaskPresence(ctx context.Context, ids []string) (map[string]bool, error) {
	return filteredTaskPresence(ids, func(batch []string) ([]string, error) {
		resp, err := q.getWithSession(ctx, "/api/v2/torrents/info?hashes="+url.QueryEscape(strings.Join(batch, "|")))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent {
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("qBittorrent 核对任务返回 HTTP %d", resp.StatusCode)
		}
		var rows []struct {
			Hash string `json:"hash"`
		}
		if err := decodeSingleJSON(resp.Body, &rows); err != nil {
			return nil, err
		}
		if rows == nil {
			return nil, errors.New("qBittorrent 未返回有效任务清单")
		}
		hashes := make([]string, 0, len(rows))
		for _, r := range rows {
			hashes = append(hashes, r.Hash)
		}
		return hashes, nil
	})
}

func (t *Transmission) TaskPresence(ctx context.Context, ids []string) (map[string]bool, error) {
	return filteredTaskPresence(ids, func(batch []string) ([]string, error) {
		resp, err := t.call(ctx, "torrent-get", map[string]any{"ids": batch, "fields": []string{"hashString"}})
		if err != nil {
			return nil, err
		}
		var payload struct {
			Torrents []struct {
				Hash string `json:"hashString"`
			} `json:"torrents"`
		}
		if err := json.Unmarshal(resp.Arguments, &payload); err != nil {
			return nil, err
		}
		if payload.Torrents == nil {
			return nil, errors.New("Transmission 未返回有效任务清单")
		}
		hashes := make([]string, 0, len(payload.Torrents))
		for _, r := range payload.Torrents {
			hashes = append(hashes, r.Hash)
		}
		return hashes, nil
	})
}
