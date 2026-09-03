package app

import (
	"context"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// InitSources 初始化所有资源源
func InitSources(cfg *config.Config) (core.Source, core.Source, core.Source) {
	mikanSource := source.NewMikanSource(cfg.Mikan.Domain, cfg.Mikan.ProxyDomain, cfg.Mikan.MirrorDomains)
	yucSource := source.NewYucWikiSource()

	extraSources := make([]core.Source, 0)
	if cfg.Sources.Nyaa.Enabled {
		extraSources = append(extraSources, source.NewNyaaSource(cfg.Sources.Nyaa.Domain))
	}
	if cfg.Sources.ACGRIP.Enabled {
		extraSources = append(extraSources, source.NewACGRIPSource(cfg.Sources.ACGRIP.Domain))
	}
	if cfg.Sources.AnimeTosho.Enabled {
		extraSources = append(extraSources, source.NewAnimeToshoSource(cfg.Sources.AnimeTosho.Domain))
	}
	
	multiSource := source.NewMultiSource(append([]core.Source{mikanSource}, extraSources...)...)
	return mikanSource, yucSource, multiSource
}

// InitMetadata 初始化元数据提供者
func InitMetadata(cfg *config.Config) core.MetadataProvider {
	var primaryMetadata core.MetadataProvider
	if cfg.Metadata.TMDB.Enabled && cfg.Metadata.TMDB.APIKey != "" {
		primaryMetadata = metadata.NewTMDBProvider(
			cfg.Metadata.TMDB.APIKey,
			cfg.Metadata.TMDB.Language,
			cfg.Metadata.TMDB.MirrorDomains,
		)
	}

	if cfg.Metadata.BGMTV.Enabled && (cfg.Metadata.BGMTV.UserToken != "" || cfg.Metadata.BGMTV.Username != "") {
		bgmProvider := metadata.NewBGMTVProvider(
			cfg.Metadata.BGMTV.UserToken,
			cfg.Metadata.BGMTV.MirrorDomains,
		)
		if cfg.Metadata.Primary == "bgmtv" || primaryMetadata == nil {
			primaryMetadata = bgmProvider
		}
	}
	return primaryMetadata
}

// InitAI 初始化 AI 客户端
func InitAI(cfg *config.Config, resolveFunc func(*config.Config) (string, string, string)) ai.Classifier {
	if !cfg.AI.Enabled {
		return nil
	}
	endpoint, apiKey, model := resolveFunc(cfg)
	protocol := ai.Protocol(cfg.AI.Protocol)
	aiClient := ai.NewClientWithBackup(endpoint, apiKey, model, cfg.AI.BackupModel, protocol)
	if aiClient.IsAvailable(context.Background()) {
		return aiClient
	}
	return nil
}
