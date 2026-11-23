package downloader

import (
	"encoding/hex"
	"math"
)

type aria2FileManifest struct {
	GID, Status, Bitfield string
	Total                 int64 `json:"totalLength,string"`
	Done                  int64 `json:"completedLength,string"`
	PieceLength           int64 `json:"pieceLength,string"`
	NumPieces             int64 `json:"numPieces,string"`
	Files                 []struct {
		Path      string
		Index     int64  `json:"index,string"`
		Length    *int64 `json:"length,string"`
		Completed *int64 `json:"completedLength,string"`
		Selected  *bool  `json:"selected,string"`
	}
}

// aria2 归档完成的任务时会丢弃逐文件完成量。此时用同一次响应中的所选文件和总完成量核实。
func (m *aria2FileManifest) confirmsSelectedBytes() bool {
	if m.Status != "complete" || m.Total <= 0 || m.Total != m.Done {
		return false
	}
	var total, selected int64
	for _, f := range m.Files {
		if f.Selected == nil || f.Length == nil || *f.Length < 0 || *f.Length > math.MaxInt64-total {
			return false
		}
		total += *f.Length
		if *f.Selected {
			selected += *f.Length
		}
	}
	if selected == 0 || selected > m.Total || m.Total > total {
		return false
	}
	if m.Bitfield == "" && m.PieceLength == 0 && m.NumPieces == 0 {
		return selected == m.Total
	}

	// 跨文件分片按整片计入完成量；仅计入所选文件需要的分片，每片只计算一次。
	if m.PieceLength <= 0 || m.NumPieces != 1+(total-1)/m.PieceLength {
		return false
	}
	if m.Bitfield == "" && selected != m.Total {
		return false
	}
	if m.Bitfield != "" && int64(len(m.Bitfield)) != 2*(1+(m.NumPieces-1)/8) {
		return false
	}
	bits, err := hex.DecodeString(m.Bitfield)
	if err != nil {
		return false
	}
	var offset, nextPiece, covered int64
	for i, f := range m.Files {
		if f.Index != int64(i)+1 {
			return false
		}
		first := offset / m.PieceLength
		offset += *f.Length
		if !*f.Selected || *f.Length == 0 {
			continue
		}
		if first < nextPiece {
			first = nextPiece
		}
		nextPiece = 1 + (offset-1)/m.PieceLength
		if first >= nextPiece {
			continue
		}
		if len(bits) > 0 {
			for p := first; p < nextPiece; p++ {
				if bits[p/8]&(0x80>>uint(p%8)) == 0 {
					return false
				}
			}
		}
		end := total
		if nextPiece < m.NumPieces {
			end = nextPiece * m.PieceLength
		}
		covered += end - first*m.PieceLength
	}
	return covered == m.Total
}
