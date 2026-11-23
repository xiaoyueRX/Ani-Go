package httpx

import (
	"errors"
	"io"
	"net/http"
	"time"
)

// WriteSSEFrame 限制慢客户端单次写入时间，帧之间保持连接。
func WriteSSEFrame(w http.ResponseWriter, frame string) error {
	rc := http.NewResponseController(w)
	err := rc.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if err == nil {
		defer rc.SetWriteDeadline(time.Time{})
	}
	if _, err := io.WriteString(w, frame); err != nil {
		return err
	}
	return rc.Flush()
}
