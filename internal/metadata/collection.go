// Package metadata 实现元数据检索、外部映射与番剧系列自动归并
package metadata

import (
	"context"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// CollectionMergePlan 单部番剧的系列归并计划建议
type CollectionMergePlan struct {
	SubscriptionID uint   `json:"subscription_id"`
	TitleCN        string `json:"title_cn"`
	OldSeriesID    string `json:"old_series_id"`
	NewSeriesID    string `json:"new_series_id"`
	CollectionName string `json:"collection_name,omitempty"`
	CollectionID   int    `json:"collection_id,omitempty"`
	Source         string `json:"source"` // tmdb_collection / tmdb_tv / normalized_franchise / cluster_merge
	ShouldApply    bool   `json:"should_apply"`
	Reason         string `json:"reason,omitempty"`
}

// CollectionMergeSummary 批量系列归并报告
type CollectionMergeSummary struct {
	TotalProcessed int                   `json:"total_processed"`
	TotalMatched   int                   `json:"total_matched"`
	TotalApplied   int                   `json:"total_applied"`
	Plans          []CollectionMergePlan `json:"plans"`
}

// CollectionMerger 提供同系列 TMDB Collection 自动识别与归并服务
type CollectionMerger struct {
	db   *gorm.DB
	tmdb *TMDBProvider
}

// NewCollectionMerger 创建系列归并服务
func NewCollectionMerger(db *gorm.DB, tmdb *TMDBProvider) *CollectionMerger {
	return &CollectionMerger{
		db:   db,
		tmdb: tmdb,
	}
}

// ResolveSingleSubscription 为单条订阅解析其 TMDB Collection 或规范化系列名称
func (m *CollectionMerger) ResolveSingleSubscription(ctx context.Context, sub *database.Subscription) (SeriesCollectionInfo, error) {
	if sub == nil {
		return SeriesCollectionInfo{}, fmt.Errorf("订阅记录为空")
	}

	title := strings.TrimSpace(sub.TitleCN)
	if title == "" {
		title = strings.TrimSpace(sub.TitleJP)
	}

	tmdbID := strings.TrimSpace(sub.TMDBID)
	if tmdbID == "" && strings.EqualFold(sub.MetadataProvider, "TMDB") {
		tmdbID = strings.TrimSpace(sub.MetadataID)
	}

	// 1. 若配置了 TMDB Provider，优先通过 TMDB 探测所属合集或 TV 系列
	if m.tmdb != nil {
		info, err := m.tmdb.ResolveSeriesCollection(ctx, tmdbID, title)
		if err == nil && info.SeriesID != "" {
			return info, nil
		}
	}

	// 2. 本地兜底：通过标题规范化提取公共 Franchise 系列名
	franchise := ExtractFranchiseTitle(title)
	if franchise != "" {
		return SeriesCollectionInfo{
			TMDBID:       NormalizeTMDBID(tmdbID),
			SeriesID:     franchise,
			FranchiseKey: NormalizeFranchiseName(franchise),
			Source:       "normalized_franchise",
		}, nil
	}

	return SeriesCollectionInfo{
		TMDBID:   NormalizeTMDBID(tmdbID),
		SeriesID: title,
		Source:   "fallback_title",
	}, nil
}

type subMeta struct {
	sub          database.Subscription
	info         SeriesCollectionInfo
	normTMDBID   string
	franchiseKey string
	oldSeries    string
}

// GenerateMergePlans 为指定订阅切片生成多维聚合的系列归并计划
// 自动通过 TMDBCollectionID、共享 SeriesID、共享 TMDBID 及规范化 Franchise 识别同系列番剧
// overwrite: 若为 false，则不覆盖用户已经手动设置的 SeriesID
func (m *CollectionMerger) GenerateMergePlans(ctx context.Context, subs []database.Subscription, overwrite bool) (CollectionMergeSummary, error) {
	summary := CollectionMergeSummary{
		TotalProcessed: len(subs),
		Plans:          make([]CollectionMergePlan, 0, len(subs)),
	}
	if len(subs) == 0 {
		return summary, nil
	}

	// 1. 解析全部订阅的单体元数据
	items := make([]subMeta, len(subs))
	for i := range subs {
		sub := subs[i]
		info, err := m.ResolveSingleSubscription(ctx, &sub)
		if err != nil {
			log.Printf("⚠️ [系列归并] 探测订阅失败 [%d: %s]: %v", sub.ID, sub.TitleCN, err)
			info = SeriesCollectionInfo{
				SeriesID: ExtractFranchiseTitle(sub.TitleCN),
				Source:   "fallback_title",
			}
		}

		normTMDB := NormalizeTMDBID(sub.TMDBID)
		if normTMDB == "" && info.TMDBID != "" {
			normTMDB = info.TMDBID
		}

		franchiseKey := info.FranchiseKey
		if franchiseKey == "" {
			franchiseKey = NormalizeFranchiseName(sub.TitleCN)
		}

		items[i] = subMeta{
			sub:          sub,
			info:         info,
			normTMDBID:   normTMDB,
			franchiseKey: franchiseKey,
			oldSeries:    strings.TrimSpace(sub.SeriesID),
		}
	}

	// 2. 查并集 (Union-Find) 进行同系列聚类
	n := len(items)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) {
		rootX := find(x)
		rootY := find(y)
		if rootX != rootY {
			parent[rootX] = rootY
		}
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if shouldGroupTogether(items[i], items[j]) {
				union(i, j)
			}
		}
	}

	// 3. 按聚类簇汇集成员并选拔统一定义的 SeriesID
	clusters := make(map[int][]int)
	for i := 0; i < n; i++ {
		root := find(i)
		clusters[root] = append(clusters[root], i)
	}

	clusterSeriesMap := make(map[int]string)
	clusterSourceMap := make(map[int]string)
	for root, memberIndices := range clusters {
		targetSeries, source := electClusterSeriesID(memberIndices, items)
		clusterSeriesMap[root] = targetSeries
		clusterSourceMap[root] = source
	}

	// 4. 生成归并计划
	for i := range items {
		item := items[i]
		root := find(i)
		newSeries := clusterSeriesMap[root]
		source := clusterSourceMap[root]
		if source == "" {
			source = item.info.Source
		}

		oldSeries := item.oldSeries
		shouldApply := false
		reason := ""

		if newSeries != "" {
			if oldSeries == "" {
				shouldApply = true
				reason = fmt.Sprintf("自动识别并归入系列: %s (来源: %s)", newSeries, source)
			} else if oldSeries != newSeries {
				if overwrite {
					shouldApply = true
					reason = fmt.Sprintf("覆盖更新为规范系列名称: %s -> %s (来源: %s)", oldSeries, newSeries, source)
				} else {
					shouldApply = false
					reason = fmt.Sprintf("已有自定义系列名称 %q，跳过覆盖", oldSeries)
				}
			} else {
				reason = "当前系列名称已保持最新规范"
			}
		}

		if newSeries != "" && (oldSeries != newSeries || oldSeries == "") {
			summary.TotalMatched++
		}

		summary.Plans = append(summary.Plans, CollectionMergePlan{
			SubscriptionID: item.sub.ID,
			TitleCN:        item.sub.TitleCN,
			OldSeriesID:    oldSeries,
			NewSeriesID:    newSeries,
			CollectionName: item.info.CollectionName,
			CollectionID:   item.info.CollectionID,
			Source:         source,
			ShouldApply:    shouldApply,
			Reason:         reason,
		})
	}

	return summary, nil
}

// shouldGroupTogether 判断两部番剧是否属于同一系列
func shouldGroupTogether(a, b subMeta) bool {
	// 条件 1: 共享非 0 的 TMDB Collection ID (剧场版合集)
	if a.info.CollectionID > 0 && a.info.CollectionID == b.info.CollectionID {
		return true
	}

	// 条件 2: 属于 TMDB 合集包含的成员
	if a.info.CollectionID > 0 && b.normTMDBID != "" {
		for _, partID := range a.info.PartTMDBIDs {
			if partID == b.normTMDBID {
				return true
			}
		}
	}
	if b.info.CollectionID > 0 && a.normTMDBID != "" {
		for _, partID := range b.info.PartTMDBIDs {
			if partID == a.normTMDBID {
				return true
			}
		}
	}

	// 条件 3: 共享非空的规范化 TMDB ID (同部 TV 的不同季或条目)
	if a.normTMDBID != "" && a.normTMDBID == b.normTMDBID {
		return true
	}

	// 条件 4: 共享已设置的非空 SeriesID
	if a.oldSeries != "" && strings.EqualFold(a.oldSeries, b.oldSeries) {
		return true
	}

	// 条件 5: 规范化系列名称 (FranchiseKey) 一致
	if a.franchiseKey != "" && a.franchiseKey == b.franchiseKey {
		return true
	}

	return false
}

// electClusterSeriesID 为同系列聚类选举统一的 SeriesID
func electClusterSeriesID(indices []int, items []subMeta) (string, string) {
	// 优先级 1: TMDB Collection 名称（官方合集定义）
	for _, idx := range indices {
		if collName := CleanCollectionName(items[idx].info.CollectionName); collName != "" {
			return collName, "tmdb_collection"
		}
	}

	// 优先级 2: 探测出的规范 SeriesID
	for _, idx := range indices {
		if s := CleanCollectionName(items[idx].info.SeriesID); s != "" && items[idx].info.Source != "fallback_title" {
			return s, items[idx].info.Source
		}
	}

	// 优先级 3: 标题主干提炼（剥离季号、篇章等）
	for _, idx := range indices {
		title := items[idx].sub.TitleCN
		if title == "" {
			title = items[idx].sub.TitleJP
		}
		if franchise := ExtractFranchiseTitle(title); franchise != "" {
			return franchise, "normalized_franchise"
		}
	}

	// 优先级 4: 若无法从标题或 TMDB 获取，继承已有定制 SeriesID
	for _, idx := range indices {
		if s := items[idx].oldSeries; s != "" {
			return s, "inherited_custom"
		}
	}

	return "", "unknown"
}

// GenerateMergePlansFromDB 从数据库查询全部订阅并生成系列归并计划
func (m *CollectionMerger) GenerateMergePlansFromDB(ctx context.Context, overwrite bool) (CollectionMergeSummary, error) {
	if m.db == nil {
		return CollectionMergeSummary{}, fmt.Errorf("数据库连接未就绪")
	}
	var subs []database.Subscription
	if err := m.db.WithContext(ctx).Find(&subs).Error; err != nil {
		return CollectionMergeSummary{}, fmt.Errorf("查询订阅列表失败: %w", err)
	}
	return m.GenerateMergePlans(ctx, subs, overwrite)
}

// ApplyMergePlans 执行系列归并计划并事务落库
func (m *CollectionMerger) ApplyMergePlans(ctx context.Context, plans []CollectionMergePlan) (int, error) {
	if m.db == nil {
		return 0, fmt.Errorf("数据库未就绪")
	}

	appliedCount := 0
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, plan := range plans {
			if !plan.ShouldApply || plan.NewSeriesID == "" {
				continue
			}

			updates := map[string]interface{}{
				"series_id": plan.NewSeriesID,
			}
			if err := tx.Model(&database.Subscription{}).Where("id = ?", plan.SubscriptionID).Updates(updates).Error; err != nil {
				return fmt.Errorf("更新订阅 [%d] 系列名称失败: %w", plan.SubscriptionID, err)
			}
			appliedCount++
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	log.Printf("📂 [系列归并] 已成功为 %d 部番剧应用统一系列归并", appliedCount)
	return appliedCount, nil
}

// AutoMergeSubscriptions 自动读取数据库订阅，执行系列归并并落库
func (m *CollectionMerger) AutoMergeSubscriptions(ctx context.Context, overwrite bool) (CollectionMergeSummary, error) {
	summary, err := m.GenerateMergePlansFromDB(ctx, overwrite)
	if err != nil {
		return summary, err
	}
	applied, err := m.ApplyMergePlans(ctx, summary.Plans)
	if err != nil {
		return summary, err
	}
	summary.TotalApplied = applied
	return summary, nil
}
