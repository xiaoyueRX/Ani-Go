package source

import (
	"testing"
)

func TestParseYucWikiSupportsLegacyAndCurrentFormats(t *testing.T) {
	html := `
		<div>周三 (10)</div>
		<a>21:00~8/12~旧版标题 环大陆</a>
		<a>21:30~ (全12话) 新版标题 大陆</a>
		<a>22:00~P1=12话 P格式标题</a>
		<div>周日 (14)</div>
		<a>09:00~9/1~另一部旧版标题</a>
		<a>10:30~(全13话) 另一部新版标题</a>
		<a>11:00~ (全12话) 第三部新版标题 大陆</a>
		<a>23:30~10/5~第三部旧版标题</a>
`

	result, err := parseYucWiki(html)
	if err != nil {
		t.Fatalf("parseYucWiki() error = %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("parsed %d weekdays, want 2", len(result))
	}

	total := 0
	parsedDates := map[string]string{
		"旧版标题": "8/12",
		"新版标题": "",
	}

	for _, day := range result {
		total += len(day.Items)
		for _, item := range day.Items {
			expectedDate, ok := parsedDates[item.Title]
			if !ok {
				continue
			}
			if item.AiredDate != expectedDate {
				t.Errorf("%q AiredDate = %q, want %q", item.Title, item.AiredDate, expectedDate)
			}
		}
	}
	if total <= 5 {
		t.Fatalf("parsed %d items, want more than 5", total)
	}
}
