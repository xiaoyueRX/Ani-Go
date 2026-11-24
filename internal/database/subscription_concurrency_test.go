package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestUpdateSubscriptionWhileFirstReadWaitsForConnection(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	if err := Init(filepath.Join(t.TempDir(), "subscriptions.db")); err != nil {
		t.Fatal(err)
	}
	db := DB
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	sub := Subscription{TitleCN: "新建订阅", Season: 1}
	if err := CreateSubscription(db, &sub); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var observed Subscription
	read := make(chan error, 1)
	err = db.Transaction(func(tx *gorm.DB) error {
		// 新订阅的后台补全会按 ID 读取订阅。让这次首次读取排在持有连接的编辑事务之后。
		before := pool.Stats().WaitCount
		go func() { read <- db.WithContext(ctx).First(&observed, sub.ID).Error }()
		for pool.Stats().WaitCount == before {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(time.Millisecond)
		}
		_, err := UpdateSubscription(tx, sub.ID, map[string]any{"total_episodes": 3})
		return err
	})
	if err != nil {
		t.Fatalf("后台补全等待连接时，编辑订阅失败: %v", err)
	}
	if err := <-read; err != nil || observed.TotalEpisodes != 3 {
		t.Fatalf("后台补全读到的总集数为 %d，错误: %v", observed.TotalEpisodes, err)
	}
}
