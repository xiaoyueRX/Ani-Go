package main

import (
	"context"
	"fmt"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

func main() {
	m := source.NewMikanSource("https://mikanime.tv", "", []string{"mikanime.tv", "mikanani.kas.pub", "mikanani.me"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	items, err := m.SearchAnime(ctx, "尖帽子的魔法工坊")
	fmt.Printf("err=%v items=%d\n", err, len(items))
	for i, it := range items {
		if i >= 3 {
			break
		}
		fmt.Printf("   %s | bid=%s\n", it.Title, it.BangumiID)
	}
}
