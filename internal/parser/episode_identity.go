package parser

// ValidEpisodeIdentity 使用与标题解析相同的季集范围；季号 0 表示特典。
func ValidEpisodeIdentity(season int, number float32) bool {
	return season >= 0 && season <= maxSeasonNumber && number > 0 && number <= maxEpisodeNumber
}
