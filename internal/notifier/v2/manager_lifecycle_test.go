package v2

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"gorm.io/gorm"
)

type lifecycleTestNotifier struct {
	name string
	send func(context.Context) error
}

func (n lifecycleTestNotifier) Name() string                                { return n.name }
func (n lifecycleTestNotifier) Send(ctx context.Context, _, _ string) error { return n.send(ctx) }

func TestStopWaitsForManualDeliveryBeforeRestart(t *testing.T) {
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	channel := lifecycleTestNotifier{"test", func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	}}
	manager := NewNotifyManager([]Notifier{channel}, event.New())
	manager.Start()
	parent, cancel := context.WithCancel(context.Background())
	var unblock sync.Once
	defer func() { cancel(); unblock.Do(func() { close(release) }); manager.Stop() }()
	result := make(chan error, 1)
	go func() { result <- manager.SendTest(parent, "test", "测试", "通知") }()
	<-entered
	stopped := make(chan struct{})
	go func() { manager.Stop(); close(stopped) }()
	select {
	case <-canceled:
	case <-stopped:
		t.Fatal("通知中心在测试投递仍运行时就报告停用")
	case <-time.After(time.Second):
		t.Fatal("停用未取消测试投递")
	}
	started := make(chan struct{})
	go func() { manager.Start(); close(started) }()
	select {
	case <-started:
		t.Fatal("旧投递没有退出，新实例就已启动")
	case <-time.After(20 * time.Millisecond):
	}
	unblock.Do(func() { close(release) })
	<-stopped
	<-started
	if err := <-result; err == nil || !manager.IsEnabled() {
		t.Fatal("取消或重启的最终状态错误")
	}
}

func TestStopFlushesAcceptedNotificationLogs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notify.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Setting{}, &database.NotificationLog{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old; sql, _ := db.DB(); sql.Close() })
	entered, release := make(chan struct{}), make(chan struct{})
	var first sync.Once
	if err := db.Callback().Create().Before("gorm:create").Register("block-first-notification", func(tx *gorm.DB) {
		if tx.Statement.Table == "notification_logs" {
			first.Do(func() { close(entered); <-release })
		}
	}); err != nil {
		t.Fatal(err)
	}
	m := NewNotifyManager(nil, nil)
	m.Start()
	const count = 24
	for i := 0; i < count; i++ {
		m.RecordManualLog("flush-on-stop", "mock", "通知", "内容", "success", "")
	}
	<-entered
	done := make(chan struct{})
	go func() { m.Stop(); close(done) }()
	deadline := time.Now().Add(time.Second)
	for m.IsEnabled() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	<-done
	var total int64
	if err := db.Model(&database.NotificationLog{}).Where("event_type = ?", "flush-on-stop").Count(&total).Error; err != nil || total != count {
		t.Fatalf("停用后已受理通知流水为 %d，预期 %d；错误 %v", total, count, err)
	}
}

func TestUnknownManualChannelDoesNotSendToPartialMatches(t *testing.T) {
	n := newRecordNotifier("telegram")
	m := NewNotifyManager([]Notifier{n}, nil)
	m.Start()
	defer m.Stop()
	if err := m.SendTest(context.Background(), "gram", "测试", "通知"); err == nil || n.count() != 0 {
		t.Fatal("无效渠道被部分匹配并投递")
	}
}

func TestSlowNotificationDoesNotBlockOtherTarget(t *testing.T) {
	slow, fast := make(chan struct{}), make(chan struct{})
	m := NewNotifyManager([]Notifier{
		lifecycleTestNotifier{"slow", func(ctx context.Context) error { close(slow); <-ctx.Done(); return ctx.Err() }},
		lifecycleTestNotifier{"fast", func(context.Context) error { close(fast); return nil }},
	}, nil)
	m.Start()
	defer m.Stop()
	m.Publish(&NotifyMessage{Target: "slow", Title: "慢速渠道"})
	<-slow
	m.Publish(&NotifyMessage{Target: "fast", Title: "独立渠道"})
	select {
	case <-fast:
	case <-time.After(time.Second):
		t.Fatal("慢速渠道阻塞了另一渠道的正常通知")
	}
}
