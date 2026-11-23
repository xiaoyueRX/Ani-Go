package backup

import (
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

func verifyArchive(raw []byte) error {
	var header struct {
		Format int    `json:"format_version"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return ErrInvalidArchive
	}
	if header.Format == 0 && header.SHA256 == "" {
		return nil
	}
	if header.Format != archiveFormat || len(header.SHA256) != sha256.Size*2 {
		return ErrInvalidArchive
	}
	var original archiveBody[json.RawMessage, json.RawMessage, json.RawMessage]
	if err := json.Unmarshal(raw, &original); err != nil {
		return ErrInvalidArchive
	}
	digest, err := archiveDigest(original)
	if err != nil || digest != header.SHA256 {
		return ErrInvalidArchive
	}
	return nil
}
