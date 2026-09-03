package source

import (
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// 模拟真实 Mikan 详情页 HTML 结构
const testDetailHTML = `<!DOCTYPE html>
<html>
<body>
<div class="content">
  <img src="/images/cover.jpg" />
</div>
<div class="bangumi-title">鬼灭之刃 游郭篇</div>
<div class="bangumi-info">
  Bangumi番组计划链接：<a href="https://bgm.tv/subject/12345">12345</a>
</div>

<div class="leftbar-item">
  <a class="subgroup-name" data-anchor="213">千夏字幕组</a>
</div>
<a name="213"></a>
<table>
  <tbody>
    <tr>
      <td><a href="/Home/Episode/1.torrent">[千夏字幕组] 鬼灭之刃 游郭篇 - 01 [1080p][CHS]</a></td>
      <td><a data-clipboard-text="magnet:?xt=urn:btih:ABCDEF1234567890ABCDEF1234567890ABCDEF12"></a></td>
      <td>1.2 GB</td>
      <td>2024-01-15</td>
    </tr>
    <tr>
      <td><a href="/Home/Episode/2.torrent">[千夏字幕组] 鬼灭之刃 游郭篇 - 02 [1080p][CHS]</a></td>
      <td><a data-clipboard-text="magnet:?xt=urn:btih:BBBB1234567890ABCDEF1234567890ABCDEF12"></a></td>
      <td>1.1 GB</td>
      <td>2024-01-22</td>
    </tr>
  </tbody>
</table>

<div class="leftbar-item">
  <a class="subgroup-name" data-anchor="456">喵萌奶茶屋</a>
</div>
<a name="456"></a>
<table>
  <tbody>
    <tr>
      <td><a href="/Home/Episode/3.torrent">[喵萌奶茶屋] 鬼灭之刃 游郭篇 - 01 [1080p][BIG5]</a></td>
      <td><a data-clipboard-text="magnet:?xt=urn:btih:CCCC1234567890ABCDEF1234567890ABCDEF12"></a></td>
      <td>1.3 GB</td>
      <td>2024-01-16</td>
    </tr>
  </tbody>
</table>
</body>
</html>`

func TestParseMikanDetailHTML_Realistic(t *testing.T) {
	items := parseMikanDetailHTML(testDetailHTML, core.Filter{}, "mikanani.me")
	if len(items) == 0 {
		t.Fatal("期望解析出种子，但返回空列表")
	}
	// 应包含两个字幕组的所有种子（共 3 个）
	if len(items) != 3 {
		t.Errorf("期望 3 个种子，实际 %d 个", len(items))
	}
}

func TestParseMikanDetailHTML_FilterBySubgroup(t *testing.T) {
	filter := core.Filter{PreferSubgroup: "千夏字幕组"}
	items := parseMikanDetailHTML(testDetailHTML, filter, "mikanani.me")
	if len(items) != 2 {
		t.Errorf("期望只返回千夏字幕组的 2 个种子，实际 %d 个", len(items))
	}
	for _, item := range items {
		// 每个条目都应该来自千夏字幕组
		info := ParseMikanTitle(item.Title)
		if info.Subgroup != "" && info.Subgroup != "千夏字幕组" {
			t.Errorf("字幕组过滤失败: 获取到 %s", info.Subgroup)
		}
	}
}

func TestParseMikanDetailHTML_Empty(t *testing.T) {
	items := parseMikanDetailHTML("", core.Filter{}, "mikanani.me")
	if items != nil {
		t.Error("空 HTML 应返回 nil")
	}
}

func TestParseMikanDetailHTML_NoTorrentTable(t *testing.T) {
	html := `<div class="leftbar-item"><a class="subgroup-name">空字幕组</a></div>`
	// 无对应表格，不应 panic
	items := parseMikanDetailHTML(html, core.Filter{}, "mikanani.me")
	if len(items) != 0 {
		t.Errorf("期望 0 个种子，实际 %d", len(items))
	}
}

const testSearchHTML = `<!DOCTYPE html>
<html>
<body>
<div class="an-ul">
  <li><a href="/Home/Bangumi/100">鬼灭之刃</a></li>
  <li><a href="/Home/Bangumi/200">迷宫饭</a></li>
  <li><a href="/Home/Bangumi/300">咒术回战</a></li>
</div>
</body>
</html>`

func TestParseMikanSearchHTML_Basic(t *testing.T) {
	items := parseMikanSearchHTML(testSearchHTML, "mikanani.me")
	if len(items) != 3 {
		t.Errorf("期望 3 个搜索结果，实际 %d", len(items))
	}
	if items[0].Title != "鬼灭之刃" {
		t.Errorf("第一个结果标题 = %q, 期望 %q", items[0].Title, "鬼灭之刃")
	}
	if items[1].Title != "迷宫饭" {
		t.Errorf("第二个结果标题 = %q, 期望 %q", items[1].Title, "迷宫饭")
	}
}

func TestParseMikanSearchHTML_Empty(t *testing.T) {
	items := parseMikanSearchHTML("", "mikanani.me")
	if items != nil {
		t.Error("空 HTML 应返回 nil")
	}
}

func TestExtractInfoHash(t *testing.T) {
	tests := []struct {
		magnet string
		want   string
	}{
		{"magnet:?xt=urn:btih:ABCDEF1234567890ABCDEF1234567890ABCDEF12", "ABCDEF1234567890ABCDEF1234567890ABCDEF12"},
		{"magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12&dn=test", "abcdef1234567890abcdef1234567890abcdef12"},
		{"", ""},
		{"not a magnet link", ""},
	}
	for _, tc := range tests {
		got := extractInfoHash(tc.magnet)
		if got != tc.want {
			t.Errorf("extractInfoHash(%q) = %q, 期望 %q", tc.magnet, got, tc.want)
		}
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1.2 GB", 1288490188},
		{"500 MB", 524288000},
		{"100 KB", 102400},
		{"2.3GB", 2469606195},
		{"", 0},
		{"unknown", 0},
		{"5 B", 5},
	}
	for _, tc := range tests {
		got := parseSize(tc.input)
		if got != tc.want {
			t.Errorf("parseSize(%q) = %d, 期望 %d", tc.input, got, tc.want)
		}
	}
}
