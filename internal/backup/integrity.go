package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const archiveFormat = 1

// 对快照的原始 JSON 字段计算摘要。恢复时使用原始字段，模型将来增加字段也能验证旧文件。
type archiveBody[T, U, V any] struct {
	Timestamp     string `json:"timestamp"`
	Version       string `json:"version"`
	Settings      T      `json:"settings"`
	Subscriptions U      `json:"subscriptions"`
	Episodes      V      `json:"episodes,omitempty"`
}

func archiveDigest(body any) (string, error) {
	h := sha256.New()
	if err := json.NewEncoder(h).Encode(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type rawArchive struct {
	archiveBody[json.RawMessage, json.RawMessage, json.RawMessage]
	Format int    `json:"format_version"`
	SHA256 string `json:"sha256"`
}

func parseArchive(raw []byte) (rawArchive, error) {
	var original rawArchive
	if err := json.Unmarshal(raw, &original); err != nil {
		return original, ErrInvalidArchive
	}
	if original.Format == 0 && original.SHA256 == "" {
		return original, nil
	}
	if original.Format != archiveFormat || len(original.SHA256) != sha256.Size*2 {
		return original, ErrInvalidArchive
	}
	digest, err := original.digest()
	if err != nil || digest != original.SHA256 {
		return original, ErrInvalidArchive
	}
	return original, nil
}

// v1 对原始 JSON 字段（包括未知属性和数字原文）计算摘要。剧集数组可能很大，
// 分段编码与一次 Encode 相同的字节，再补上 Encode 的最后一个换行。
func (original rawArchive) digest() (string, error) {
	body := original.archiveBody
	episodes := body.Episodes
	if len(episodes) == 0 || episodes[0] != '[' {
		return archiveDigest(body)
	}
	body.Episodes = nil
	prefix, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	sum.Write(prefix[:len(prefix)-1]) // 数组接在原对象的右括号之前。
	sum.Write([]byte(`,"episodes":[`))
	dec := json.NewDecoder(bytes.NewReader(episodes))
	if _, err := dec.Token(); err != nil {
		return "", err
	}
	var row json.RawMessage
	for first := true; dec.More(); first = false {
		if err := dec.Decode(&row); err != nil {
			return "", err
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return "", err
		}
		if !first {
			sum.Write([]byte(","))
		}
		sum.Write(encoded)
	}
	sum.Write([]byte("]}\n"))
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func verifyArchive(raw []byte) error {
	_, err := parseArchive(raw)
	return err
}
