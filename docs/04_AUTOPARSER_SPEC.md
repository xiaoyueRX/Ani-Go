# 订阅资源的季集识别

本页描述 RSS 和历史补全共用的订阅解析路径。自然语言指令使用单独的任务解析器。[订阅解析入口](../internal/scheduler/subscription_feeds.go) · [任务解析器](../internal/parser)

多作品 RSS 和历史搜索按完整片名及中、日、英别名匹配。竖线或两侧留白的斜线可分隔译名；无空格的斜线要求两侧都与订阅别名相符。已有 Bangumi.tv 元数据 ID 的订阅会核对蜜柑详情页的条目链接，找不到对应关系时保留订阅，继续按片名查询支持搜索的来源。

| 输入或条件 | 调度器如何处理 |
| --- | --- |
| 标题带明确季号、特殊篇或合集标记，或命中自定义规则 | 保留本地解析结果。未明确标季的常规标题可采用订阅季度。 |
| 集号未识别，或后续季度的集号超出已知本季总集数 | 查询按订阅 ID、季度和原始标题区分的持久化消歧缓存。 |
| 缓存未命中，有发布时间、元数据 ID，且订阅的元数据来源与当前提供者相符 | 获取分集放送数据，用时间窗口和标题中的集号线索判定候选。缺少元数据时继续下一步。 |
| 仍有歧义，开启 `AI_AUTO_DISAMBIGUATE` 且 AI 客户端可用 | 结合原始标题、订阅名及发布时间调用 AI。仅接受季集在允许范围且置信度至少 0.85 的结果，成功后缓存。其余情况保留本地结果。 |

默认播出匹配窗口允许提前 6 小时与延后 72 小时。日期与标题线索用于处理例如「第 2 季、第 25 话」这样的绝对集号；解析器不会因发布时间较晚就改写普通历史重发的集号。[日期匹配](../internal/parser/airdate_matcher.go) · [缓存](../internal/parser/cache.go)

`AI_DAILY_QUOTA_LIMIT` 控制后台消歧的每日**调用次数**，默认 30。计数在进程内维护，重启会重置；SQLite 中的成功解析结果可跨重启复用。[调用配额](../internal/ai/quota.go) · [AI 请求](../internal/ai/client.go)

## Episode matching in brief

Shared RSS feeds and history searches match whole titles and their Chinese, Japanese and English aliases. When a subscription has a Bangumi.tv subject ID, automatic Mikan linking checks the subject linked from the Mikan detail page. An unverified subscription can still search other sources by title.

RSS and historical supplements use the same subscription resolver. Explicit season numbers, specials, batches and custom matches retain their local result. Missing or out-of-range episode numbers may use a cached match, then the subscription's matching metadata provider and publication time. The default air-date window runs from six hours before to 72 hours after broadcast. If enabled, AI can resolve remaining cases at confidence 0.85 or higher within the allowed season and episode range. Otherwise the resolver returns the local result. Successful matches are cached by subscription, season and original title. The optional daily AI limit counts calls in the running process and defaults to 30. [Resolver](../internal/scheduler/subscription_feeds.go)
