package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const searchInterval = 2 * time.Second

func main() {
	apply := flag.Bool("apply", false, "write matched bangumi_id values to the database")
	flag.Parse()

	db, err := gorm.Open(sqlite.Open("data/ani-go.db?_pragma=busy_timeout(5000)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	var subscriptions []database.Subscription
	if err := db.Where("bangumi_id = '' AND deleted_at IS NULL").Order("id").Find(&subscriptions).Error; err != nil {
		log.Fatalf("查询订阅失败: %v", err)
	}
	if len(subscriptions) == 0 {
		fmt.Println("没有需要回填的订阅")
		return
	}

	mikan := source.NewMikanSource("https://mikanime.tv", "", []string{"mikanime.tv", "mikanani.kas.pub", "mikanani.me"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Println("订阅ID | 标题 | 匹配到的bangumi_id | 置信度")
	for index, item := range subscriptions {
		if index > 0 {
			time.Sleep(searchInterval)
		}
		if results, err := mikan.SearchAnime(ctx, item.TitleCN); err != nil {
			fmt.Printf("%d | %s | 搜索失败: %v | 0.00\n", item.ID, item.TitleCN, err)
			continue
		} else {
			bangumiID, confidence := matchBangumiID(item, results)
			fmt.Printf("%d | %s | %s | %.2f\n", item.ID, item.TitleCN, bangumiID, confidence)
			if *apply && bangumiID != "" {
				if err := db.Model(&database.Subscription{}).Where("id = ?", item.ID).Update("bangumi_id", bangumiID).Error; err != nil {
					log.Fatalf("更新订阅 %d 失败: %v", item.ID, err)
				}
			}
		}
	}

	if !*apply {
		fmt.Println("DRY RUN: 未修改数据库")
	}
}

func matchBangumiID(sub database.Subscription, results []core.TorrentItem) (string, float64) {
	targets := make([]string, 0, 3)
	for _, title := range []string{sub.TitleCN, sub.TitleEN, sub.TitleJP} {
		if normalized := normalizeTitle(title); normalized != "" {
			targets = append(targets, normalized)
		}
	}
	for _, result := range results {
		resultTitle := normalizeTitle(result.Title)
		for _, target := range targets {
			if resultTitle == target {
				return result.BangumiID, 1.00
			}
		}
	}
	return "", 0
}

func normalizeTitle(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, char := range strings.TrimSpace(value) {
		switch {
		case unicode.Is(unicode.Han, char), unicode.Is(unicode.Katakana, char),
			unicode.Is(unicode.Hiragana, char), unicode.Is(unicode.Hangul, char):
			builder.WriteRune(char)
		case unicode.IsLetter(char), unicode.IsDigit(char):
			builder.WriteRune(unicode.ToLower(char))
		}
	}
	return builder.String()
}
