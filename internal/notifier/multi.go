package notifier

import (
	"context"
	"log"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type MultiNotifier struct {
	notifiers []core.Notifier
}

func NewMultiNotifier(notifiers ...core.Notifier) *MultiNotifier {
	filtered := make([]core.Notifier, 0, len(notifiers))
	for _, n := range notifiers {
		if n != nil {
			filtered = append(filtered, n)
		}
	}
	mn := &MultiNotifier{notifiers: filtered}
	if len(filtered) > 0 {
		names := make([]string, len(filtered))
		for i, n := range filtered {
			names[i] = n.Name()
		}
		log.Printf("🔔 MultiNotifier 已聚合 %d 个通知渠道: %v", len(filtered), names)
	}
	return mn
}

func (mn *MultiNotifier) Name() string { return "MultiNotifier" }

func (mn *MultiNotifier) Send(ctx context.Context, title, message string) error {
	for _, n := range mn.notifiers {
		go func(notifier core.Notifier) {
			if err := notifier.Send(ctx, title, message); err != nil {
				log.Printf("⚠️  [%s] 通知发送失败: %v", notifier.Name(), err)
			}
		}(n)
	}
	return nil
}

func (mn *MultiNotifier) Add(n core.Notifier) {
	mn.notifiers = append(mn.notifiers, n)
}

func (mn *MultiNotifier) Count() int {
	return len(mn.notifiers)
}
