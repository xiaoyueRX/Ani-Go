// Package ai 提供 AI 辅助功能（可选模块）
package ai

import (
	"context"
	"fmt"
	"log"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries int
	Backoff    []time.Duration
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries: 2,
	Backoff:    []time.Duration{100 * time.Millisecond, 300 * time.Millisecond},
}

// RetryableError 可重试的错误接口
type RetryableError interface {
	error
	Temporary() bool
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	// 检查是否实现了 Temporary() 接口
	if re, ok := err.(RetryableError); ok {
		return re.Temporary()
	}
	// 网络错误、超时、5xx 错误通常可重试
	errStr := err.Error()
	return containsAny(errStr, []string{
		"timeout",
		"connection refused",
		"connection reset",
		"no such host",
		"500",
		"502",
		"503",
		"504",
	})
}

// WithRetry 执行带重试的操作
func WithRetry(ctx context.Context, config RetryConfig, fn func() (string, error)) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("🔁 AI 请求重试 (%d/%d): %v", attempt, config.MaxRetries, lastErr)
			select {
			case <-time.After(config.Backoff[attempt-1]):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		resp, err := fn()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// 如果错误不可重试，直接返回
		if !IsRetryable(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("重试 %d 次后仍失败: %w", config.MaxRetries, lastErr)
}

// WithFallback 执行带备用模型的操作
func WithFallback(ctx context.Context, primaryFn, fallbackFn func() (string, error), primaryName, fallbackName string) (string, error) {
	resp, err := primaryFn()
	if err == nil {
		return resp, nil
	}

	log.Printf("⚠️ 主模型 %s 失败(%v)，切换备用模型 %s", primaryName, err, fallbackName)
	resp, err = fallbackFn()
	if err == nil {
		return resp, nil
	}
	return "", fmt.Errorf("主模型与备用模型均失败: 主=%v, 备=%w", err, err)
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	failures    int
	maxFailures int
	lastFailure time.Time
	open        bool
	halfOpen    bool
}

func NewCircuitBreaker(maxFailures int) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: maxFailures,
	}
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() (string, error)) (string, error) {
	if cb.open {
		// 检查是否可以尝试半开状态
		if time.Since(cb.lastFailure) > 30*time.Second {
			cb.halfOpen = true
			cb.open = false
		} else {
			return "", fmt.Errorf("熔断器开启，拒绝请求")
		}
	}

	resp, err := fn()
	if err != nil {
		cb.recordFailure()
		return "", err
	}

	cb.recordSuccess()
	return resp, nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.maxFailures {
		cb.open = true
		cb.halfOpen = false
		log.Printf("🔴 熔断器开启，失败次数: %d", cb.failures)
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	if cb.halfOpen {
		// 半开状态下成功，重置熔断器
		cb.failures = 0
		cb.open = false
		cb.halfOpen = false
		log.Printf("🟢 熔断器关闭（半开探测成功）")
	} else if cb.failures > 0 {
		cb.failures--
	}
}

// containsAny 检查字符串是否包含任一子串
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}