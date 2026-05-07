package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := defaults()
	if cfg.Server.Port != 20001 {
		t.Errorf("默认端口 = %d, 期望 20001", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("默认 Host = %s, 期望 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Database.Path != "ani-go.db" {
		t.Errorf("数据库路径 = %s", cfg.Database.Path)
	}
	if cfg.Mikan.Domain != "mikanime.tv" {
		t.Errorf("Mikan 域名 = %s, 期望 mikanime.tv", cfg.Mikan.Domain)
	}
	if len(cfg.Mikan.MirrorDomains) != 2 {
		t.Errorf("Mikan 镜像数 = %d, 期望 2", len(cfg.Mikan.MirrorDomains))
	}
	if cfg.Downloaders.Default != "qbittorrent" {
		t.Errorf("默认下载器 = %s", cfg.Downloaders.Default)
	}
	if cfg.Downloaders.QBittorrent.Host != "http://localhost:8081" {
		t.Errorf("QB Host = %s", cfg.Downloaders.QBittorrent.Host)
	}
	if cfg.Downloaders.QBittorrent.Category != "ani-go" {
		t.Errorf("QB Category = %s", cfg.Downloaders.QBittorrent.Category)
	}
	if cfg.Metadata.Primary != "tmdb" {
		t.Errorf("主元数据源 = %s", cfg.Metadata.Primary)
	}
	if cfg.Metadata.TMDB.Language != "zh-CN" {
		t.Errorf("TMDB 语言 = %s", cfg.Metadata.TMDB.Language)
	}
	if !cfg.Metadata.BGMTV.Enabled {
		t.Error("BGM.tv 应默认启用")
	}
	if cfg.Organizer.TVBasePath != "./TV/番剧" {
		t.Errorf("TV 路径 = %s", cfg.Organizer.TVBasePath)
	}
	if cfg.Organizer.MovieBasePath != "./TV/剧场版" {
		t.Errorf("Movie 路径 = %s", cfg.Organizer.MovieBasePath)
	}
	if cfg.Organizer.UseHardLink {
		t.Error("硬链接应默认禁用")
	}
	if cfg.AI.Enabled {
		t.Error("AI 应默认禁用")
	}
	if cfg.Scheduler.RSSInterval == 0 {
		t.Error("RSS 间隔不应为 0")
	}
	if cfg.Scheduler.OrganizerInterval == 0 {
		t.Error("整理间隔不应为 0")
	}
}

func TestLoad_ServerPort(t *testing.T) {
	os.Setenv("PORT", "8080")
	defer os.Unsetenv("PORT")

	cfg := Load()
	if cfg.Server.Port != 8080 {
		t.Errorf("端口 = %d, 期望 8080", cfg.Server.Port)
	}
}

func TestLoad_InvalidPortFallsBack(t *testing.T) {
	os.Setenv("PORT", "abc")
	defer os.Unsetenv("PORT")

	cfg := Load()
	if cfg.Server.Port != 20001 {
		t.Errorf("非法端口应回退默认值 20001, 实际: %d", cfg.Server.Port)
	}
}

func TestLoad_DBPath(t *testing.T) {
	os.Setenv("DB_PATH", "/custom/path.db")
	defer os.Unsetenv("DB_PATH")

	cfg := Load()
	if cfg.Database.Path != "/custom/path.db" {
		t.Errorf("数据库路径 = %s", cfg.Database.Path)
	}
}

func TestLoad_MikanConfig(t *testing.T) {
	os.Setenv("MIKAN_RSS_URL", "https://mikanani.me/RSS/MyBangumi?token=abc")
	os.Setenv("MIKAN_DOMAIN", "mikanime.tv")
	os.Setenv("MIKAN_PROXY_DOMAIN", "proxy.example.com")
	os.Setenv("MIKAN_MIRROR_DOMAINS", "a.mikan.me,b.mikan.me")
	defer os.Unsetenv("MIKAN_RSS_URL")
	defer os.Unsetenv("MIKAN_DOMAIN")
	defer os.Unsetenv("MIKAN_PROXY_DOMAIN")
	defer os.Unsetenv("MIKAN_MIRROR_DOMAINS")

	cfg := Load()
	if cfg.Mikan.PersonalRSSURL != "https://mikanani.me/RSS/MyBangumi?token=abc" {
		t.Errorf("RSS URL = %s", cfg.Mikan.PersonalRSSURL)
	}
	if cfg.Mikan.Domain != "mikanime.tv" {
		t.Errorf("域名 = %s", cfg.Mikan.Domain)
	}
	if cfg.Mikan.ProxyDomain != "proxy.example.com" {
		t.Errorf("代理域名 = %s", cfg.Mikan.ProxyDomain)
	}
	if len(cfg.Mikan.MirrorDomains) != 2 {
		t.Errorf("镜像数 = %d, 期望 2", len(cfg.Mikan.MirrorDomains))
	}
}

func TestLoad_QBitTorrent(t *testing.T) {
	os.Setenv("QB_HOST", "http://192.168.1.100:8081")
	os.Setenv("QB_USER", "admin")
	os.Setenv("QB_PASS", "pass123")
	defer os.Unsetenv("QB_HOST")
	defer os.Unsetenv("QB_USER")
	defer os.Unsetenv("QB_PASS")

	cfg := Load()
	if !cfg.Downloaders.QBittorrent.Enabled {
		t.Error("QB 应自动启用")
	}
	if cfg.Downloaders.QBittorrent.Host != "http://192.168.1.100:8081" {
		t.Errorf("QB Host = %s", cfg.Downloaders.QBittorrent.Host)
	}
	if cfg.Downloaders.QBittorrent.Username != "admin" {
		t.Errorf("QB User = %s", cfg.Downloaders.QBittorrent.Username)
	}
	if cfg.Downloaders.QBittorrent.Password != "pass123" {
		t.Errorf("QB Pass = %s", cfg.Downloaders.QBittorrent.Password)
	}
}

func TestLoad_Transmission(t *testing.T) {
	os.Setenv("TR_HOST", "http://localhost:9091")
	os.Setenv("TR_USER", "truser")
	os.Setenv("TR_PASS", "trpass")
	defer os.Unsetenv("TR_HOST")
	defer os.Unsetenv("TR_USER")
	defer os.Unsetenv("TR_PASS")

	cfg := Load()
	if !cfg.Downloaders.Transmission.Enabled {
		t.Error("Transmission 应自动启用")
	}
	if cfg.Downloaders.Transmission.Host != "http://localhost:9091" {
		t.Errorf("TR Host = %s", cfg.Downloaders.Transmission.Host)
	}
}

func TestLoad_DefaultDownloader(t *testing.T) {
	os.Setenv("DEFAULT_DOWNLOADER", "transmission")
	defer os.Unsetenv("DEFAULT_DOWNLOADER")

	cfg := Load()
	if cfg.Downloaders.Default != "transmission" {
		t.Errorf("默认下载器 = %s, 期望 transmission", cfg.Downloaders.Default)
	}
}

func TestLoad_Aria2(t *testing.T) {
	os.Setenv("ARIA2_HOST", "http://localhost:6800")
	os.Setenv("ARIA2_SECRET", "mysecret")
	defer os.Unsetenv("ARIA2_HOST")
	defer os.Unsetenv("ARIA2_SECRET")

	cfg := Load()
	if !cfg.Downloaders.Aria2.Enabled {
		t.Error("Aria2 应自动启用")
	}
	if cfg.Downloaders.Aria2.Secret != "mysecret" {
		t.Errorf("Aria2 Secret = %s", cfg.Downloaders.Aria2.Secret)
	}
}

func TestLoad_TMDB(t *testing.T) {
	os.Setenv("TMDB_API_KEY", "key123")
	os.Setenv("TMDB_MIRROR_DOMAINS", "tmdb.example.com,tmdb2.example.com")
	defer os.Unsetenv("TMDB_API_KEY")
	defer os.Unsetenv("TMDB_MIRROR_DOMAINS")

	cfg := Load()
	if !cfg.Metadata.TMDB.Enabled {
		t.Error("TMDB 应自动启用")
	}
	if cfg.Metadata.TMDB.APIKey != "key123" {
		t.Errorf("TMDB Key = %s", cfg.Metadata.TMDB.APIKey)
	}
	if len(cfg.Metadata.TMDB.MirrorDomains) != 2 {
		t.Errorf("TMDB 镜像数 = %d", len(cfg.Metadata.TMDB.MirrorDomains))
	}
}

func TestLoad_BGMTV(t *testing.T) {
	os.Setenv("BGMTV_USER_TOKEN", "bgtoken")
	defer os.Unsetenv("BGMTV_USER_TOKEN")

	cfg := Load()
	if !cfg.Metadata.BGMTV.Enabled {
		t.Error("BGM.tv 应自动启用")
	}
	if cfg.Metadata.BGMTV.UserToken != "bgtoken" {
		t.Errorf("BGM Token = %s", cfg.Metadata.BGMTV.UserToken)
	}
}

func TestLoad_OrganizerPaths(t *testing.T) {
	os.Setenv("TV_BASE_PATH", "/media/tv")
	os.Setenv("MOVIE_BASE_PATH", "/media/movie")
	os.Setenv("OVA_BASE_PATH", "/media/ova")
	defer os.Unsetenv("TV_BASE_PATH")
	defer os.Unsetenv("MOVIE_BASE_PATH")
	defer os.Unsetenv("OVA_BASE_PATH")

	cfg := Load()
	if cfg.Organizer.TVBasePath != "/media/tv" {
		t.Errorf("TV = %s", cfg.Organizer.TVBasePath)
	}
	if cfg.Organizer.MovieBasePath != "/media/movie" {
		t.Errorf("Movie = %s", cfg.Organizer.MovieBasePath)
	}
	if cfg.Organizer.OVABasePath != "/media/ova" {
		t.Errorf("OVA = %s", cfg.Organizer.OVABasePath)
	}
}

func TestLoad_AIConfig(t *testing.T) {
	os.Setenv("AI_PROTOCOL", "openai")
	os.Setenv("AI_ENDPOINT", "https://api.openai.com/v1/chat/completions")
	os.Setenv("AI_API_KEY", "sk-test")
	os.Setenv("AI_MODEL", "gpt-4")
	defer os.Unsetenv("AI_PROTOCOL")
	defer os.Unsetenv("AI_ENDPOINT")
	defer os.Unsetenv("AI_API_KEY")
	defer os.Unsetenv("AI_MODEL")

	cfg := Load()
	if cfg.AI.Protocol != "openai" {
		t.Errorf("协议 = %s", cfg.AI.Protocol)
	}
	if !cfg.AI.Enabled {
		t.Error("AI 应自动启用")
	}
	if cfg.AI.APIKey != "sk-test" {
		t.Errorf("API Key = %s", cfg.AI.APIKey)
	}
	if cfg.AI.Model != "gpt-4" {
		t.Errorf("模型 = %s", cfg.AI.Model)
	}
}

func TestLoad_Sources(t *testing.T) {
	os.Setenv("NYAA_DOMAIN", "nyaa.si")
	os.Setenv("ACGRIP_DOMAIN", "acg.rip")
	os.Setenv("ANIMETOSHO_DOMAIN", "feed.animetosho.org")
	defer os.Unsetenv("NYAA_DOMAIN")
	defer os.Unsetenv("ACGRIP_DOMAIN")
	defer os.Unsetenv("ANIMETOSHO_DOMAIN")

	cfg := Load()
	if !cfg.Sources.Nyaa.Enabled {
		t.Error("Nyaa 应启用")
	}
	if !cfg.Sources.ACGRIP.Enabled {
		t.Error("ACGRIP 应启用")
	}
	if !cfg.Sources.AnimeTosho.Enabled {
		t.Error("AnimeTosho 应启用")
	}
}

func TestLoad_Notifiers(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "bot123")
	os.Setenv("TELEGRAM_CHAT_ID", "chat456")
	os.Setenv("DISCORD_WEBHOOK", "https://discord.com/api/webhooks/x")
	os.Setenv("WECOM_WEBHOOK", "https://qyapi.weixin.qq.com/webhook/x")
	os.Setenv("FEISHU_WEBHOOK", "https://open.feishu.cn/hook/x")
	os.Setenv("DINGTALK_WEBHOOK", "https://oapi.dingtalk.com/robot/x")
	os.Setenv("LINE_CHANNEL_TOKEN", "line-token")
	os.Setenv("LINE_USER_ID", "U123")
	os.Setenv("WHATSAPP_PHONE_ID", "123456")
	os.Setenv("WHATSAPP_TOKEN", "wa-token")
	os.Setenv("WHATSAPP_TO", "8613800138000")
	defer func() {
		for _, k := range []string{
			"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "DISCORD_WEBHOOK",
			"WECOM_WEBHOOK", "FEISHU_WEBHOOK", "DINGTALK_WEBHOOK",
			"LINE_CHANNEL_TOKEN", "LINE_USER_ID",
			"WHATSAPP_PHONE_ID", "WHATSAPP_TOKEN", "WHATSAPP_TO",
		} {
			os.Unsetenv(k)
		}
	}()

	cfg := Load()
	if cfg.Notifier.TelegramBotToken != "bot123" {
		t.Errorf("Telegram Token = %s", cfg.Notifier.TelegramBotToken)
	}
	if cfg.Notifier.TelegramChatID != "chat456" {
		t.Errorf("Telegram ChatID = %s", cfg.Notifier.TelegramChatID)
	}
	if cfg.Notifier.DiscordWebhook != "https://discord.com/api/webhooks/x" {
		t.Errorf("Discord = %s", cfg.Notifier.DiscordWebhook)
	}
	if cfg.Notifier.LINEChannelToken != "line-token" {
		t.Errorf("LINE Token = %s", cfg.Notifier.LINEChannelToken)
	}
	if cfg.Notifier.LINEUserID != "U123" {
		t.Errorf("LINE UserID = %s", cfg.Notifier.LINEUserID)
	}
	if cfg.Notifier.WhatsAppPhoneID != "123456" {
		t.Errorf("WhatsApp PhoneID = %s", cfg.Notifier.WhatsAppPhoneID)
	}
}

func TestLoad_OneBotInt64(t *testing.T) {
	os.Setenv("ONEBOT_HOST", "http://localhost:3000")
	os.Setenv("ONEBOT_USER_ID", "123456789")
	os.Setenv("ONEBOT_GROUP_ID", "987654321")
	defer os.Unsetenv("ONEBOT_HOST")
	defer os.Unsetenv("ONEBOT_USER_ID")
	defer os.Unsetenv("ONEBOT_GROUP_ID")

	cfg := Load()
	if cfg.Notifier.OneBotHost != "http://localhost:3000" {
		t.Errorf("OneBot Host = %s", cfg.Notifier.OneBotHost)
	}
	if cfg.Notifier.OneBotUserID != 123456789 {
		t.Errorf("OneBot UserID = %d", cfg.Notifier.OneBotUserID)
	}
	if cfg.Notifier.OneBotGroupID != 987654321 {
		t.Errorf("OneBot GroupID = %d", cfg.Notifier.OneBotGroupID)
	}
}

func TestLoad_Matrix(t *testing.T) {
	os.Setenv("MATRIX_HOMESERVER", "https://matrix.org")
	os.Setenv("MATRIX_TOKEN", "syt_test")
	os.Setenv("MATRIX_ROOM_ID", "!room:matrix.org")
	defer os.Unsetenv("MATRIX_HOMESERVER")
	defer os.Unsetenv("MATRIX_TOKEN")
	defer os.Unsetenv("MATRIX_ROOM_ID")

	cfg := Load()
	if cfg.Notifier.MatrixToken != "syt_test" {
		t.Errorf("Matrix Token = %s", cfg.Notifier.MatrixToken)
	}
}

func TestLoad_Email(t *testing.T) {
	os.Setenv("EMAIL_SMTP_HOST", "smtp.gmail.com")
	os.Setenv("EMAIL_SMTP_PORT", "587")
	os.Setenv("EMAIL_USERNAME", "test@gmail.com")
	os.Setenv("EMAIL_TO", "admin@ex.com,user@ex.com")
	defer func() {
		for _, k := range []string{"EMAIL_SMTP_HOST", "EMAIL_SMTP_PORT", "EMAIL_USERNAME", "EMAIL_TO"} {
			os.Unsetenv(k)
		}
	}()

	cfg := Load()
	if cfg.Notifier.EmailSMTPHost != "smtp.gmail.com" {
		t.Errorf("SMTP Host = %s", cfg.Notifier.EmailSMTPHost)
	}
	if cfg.Notifier.EmailTo != "admin@ex.com,user@ex.com" {
		t.Errorf("Email To = %s", cfg.Notifier.EmailTo)
	}
}

func TestSplitEnv_Comma(t *testing.T) {
	result := splitEnv("a,b,c")
	if len(result) != 3 {
		t.Fatalf("len = %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("result = %v", result)
	}
}

func TestSplitEnv_Space(t *testing.T) {
	result := splitEnv("x y z")
	if len(result) != 3 {
		t.Fatalf("len = %d", len(result))
	}
}

func TestSplitEnv_Mixed(t *testing.T) {
	result := splitEnv("a.com, b.com c.com")
	if len(result) != 3 {
		t.Fatalf("len = %d, result = %v", len(result), result)
	}
}

func TestSplitEnv_Empty(t *testing.T) {
	result := splitEnv("")
	if len(result) != 0 {
		t.Fatalf("len = %d", len(result))
	}
}

func TestSplitEnv_TrailingComma(t *testing.T) {
	result := splitEnv("a.com,")
	if len(result) != 1 {
		t.Fatalf("len = %d", len(result))
	}
	if result[0] != "a.com" {
		t.Errorf(" = %s", result[0])
	}
}
