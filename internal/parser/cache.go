package parser

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

const (
	// maxCacheTitleRunes 缓存标题最大字符长度，超出截断以防止超长脏数据撑爆数据库
	maxCacheTitleRunes = 1000
	// minCacheConfidence 允许写入持久化缓存的最低置信度阈值（防毒策略）
	minCacheConfidence = 0.2
)

var (
	// cacheWriteMu 互斥锁，保障 SQLite 并发写入时杜绝 FirstOrCreate 唯一索引冲突
	cacheWriteMu sync.Mutex
)

// CachedTitleResult 缓存的种子标题结构化解析数据
type CachedTitleResult struct {
	RawTitle   string
	CleanTitle string
	Season     int
	Episode    float32
	Subgroup   string
	Resolution string
	Confidence float32
	ResolvedBy string
	CreatedAt  time.Time
}

// NormalizeCacheTitle 规范化缓存标题字符串（清洗控制字符、剥离首尾空白并实施长度保护）
func NormalizeCacheTitle(rawTitle string) string {
	s := sanitizeControlRunes(rawTitle)
	s = truncateRunes(s, maxCacheTitleRunes)
	return strings.TrimSpace(s)
}

// GetCachedTitleResult 从持久化数据库缓存中读取标题解析结果
func GetCachedTitleResult(rawTitle string) (*CachedTitleResult, bool) {
	if database.DB == nil {
		return nil, false
	}
	title := NormalizeCacheTitle(rawTitle)
	if title == "" {
		return nil, false
	}

	var record database.ParserCache
	if err := database.DB.Where("raw_title = ?", title).First(&record).Error; err != nil {
		return nil, false
	}

	return &CachedTitleResult{
		RawTitle:   record.RawTitle,
		CleanTitle: record.CleanTitle,
		Season:     record.Season,
		Episode:    record.Episode,
		Subgroup:   record.Subgroup,
		Resolution: record.Resolution,
		Confidence: record.Confidence,
		ResolvedBy: record.ResolvedBy,
		CreatedAt:  record.CreatedAt,
	}, true
}

// SaveCachedTitleResult 将解析结果保存到持久化数据库中（防毒策略：拒绝缓存解析失败或零置信度条目）
func SaveCachedTitleResult(rawTitle, cleanTitle string, season int, episode float32, subgroup, resolution string, confidence float32, resolvedBy string) error {
	if database.DB == nil {
		return nil
	}

	title := NormalizeCacheTitle(rawTitle)
	if title == "" {
		return nil
	}

	clean := NormalizeCacheTitle(cleanTitle)

	// 防毒策略 (Cache Poisoning Prevention)：
	// 1. 置信度为 NaN/Inf 或低于阈值（< 0.2）时不存入缓存；
	// 2. 标题完全为空且无有效集数时不存入缓存；
	// 3. 季号或集数出现非法负值时不存入缓存。
	conf64 := float64(confidence)
	if math.IsNaN(conf64) || math.IsInf(conf64, 0) || confidence < minCacheConfidence {
		return nil
	}
	if clean == "" && episode <= 0 {
		return nil
	}
	if season < 0 || season > maxSeasonNumber || episode < 0 || episode > maxEpisodeNumber {
		return nil
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	// 并发写保护：加锁后先查后更/建，根除 SQLite 下高并发导致的 UNIQUE constraint 报错
	cacheWriteMu.Lock()
	defer cacheWriteMu.Unlock()

	var existing database.ParserCache
	err := database.DB.Where("raw_title = ?", title).First(&existing).Error
	if err == nil {
		// 存在旧缓存：更新解析结果
		return database.DB.Model(&existing).Updates(map[string]interface{}{
			"clean_title": clean,
			"season":      season,
			"episode":     episode,
			"subgroup":    truncateRunes(strings.TrimSpace(subgroup), 128),
			"resolution":  truncateRunes(strings.ToLower(strings.TrimSpace(resolution)), 32),
			"confidence":  confidence,
			"resolved_by": truncateRunes(strings.TrimSpace(resolvedBy), 32),
		}).Error
	}

	if !errors.Is(err, database.DB.Error) {
		record := database.ParserCache{
			RawTitle:   title,
			CleanTitle: clean,
			Season:     season,
			Episode:    episode,
			Subgroup:   truncateRunes(strings.TrimSpace(subgroup), 128),
			Resolution: truncateRunes(strings.ToLower(strings.TrimSpace(resolution)), 32),
			Confidence: confidence,
			ResolvedBy: truncateRunes(strings.TrimSpace(resolvedBy), 32),
		}
		return database.DB.Create(&record).Error
	}

	return err
}

// DeleteCachedTitleResult 从数据库中删除指定标题的缓存记录
func DeleteCachedTitleResult(rawTitle string) error {
	if database.DB == nil {
		return nil
	}
	title := NormalizeCacheTitle(rawTitle)
	if title == "" {
		return nil
	}
	cacheWriteMu.Lock()
	defer cacheWriteMu.Unlock()
	return database.DB.Where("raw_title = ?", title).Delete(&database.ParserCache{}).Error
}

// ClearParserCache 清空全部持久化标题缓存
func ClearParserCache() error {
	if database.DB == nil {
		return nil
	}
	cacheWriteMu.Lock()
	defer cacheWriteMu.Unlock()
	return database.DB.Exec("DELETE FROM parser_caches").Error
}
