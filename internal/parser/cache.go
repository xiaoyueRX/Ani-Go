package parser

import (
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
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

// GetCachedTitleResult 从持久化数据库缓存中读取标题解析结果
func GetCachedTitleResult(rawTitle string) (*CachedTitleResult, bool) {
	if database.DB == nil {
		return nil, false
	}
	title := strings.TrimSpace(rawTitle)
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

// SaveCachedTitleResult 将解析结果保存到持久化数据库中（终身唯一缓存，防止二次消耗）
func SaveCachedTitleResult(rawTitle, cleanTitle string, season int, episode float32, subgroup, resolution string, confidence float32, resolvedBy string) error {
	if database.DB == nil {
		return nil
	}
	title := strings.TrimSpace(rawTitle)
	if title == "" {
		return nil
	}

	record := database.ParserCache{
		RawTitle:   title,
		CleanTitle: cleanTitle,
		Season:     season,
		Episode:    episode,
		Subgroup:   subgroup,
		Resolution: resolution,
		Confidence: confidence,
		ResolvedBy: resolvedBy,
	}

	return database.DB.Where("raw_title = ?", title).Assign(record).FirstOrCreate(&record).Error
}
