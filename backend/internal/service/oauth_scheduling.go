package service

import (
	"math"
)

// oauth7dWindowSeconds 定义 OAuth 账号 7 天用量窗口的总秒数（7 * 24 * 3600 = 604800 秒）。
const oauth7dWindowSeconds = int64(7 * 24 * 3600)

// calculateOAuth7dSchedulingScore 计算 OAuth 账号 7 天窗口结合用量百分比与距离重置时间的综合调度评分（数值越小越优先）。
//
// 业务背景与算法原理：
// 若单纯按用量百分比排序（已用更少优先），会导致距离重置时间极短（如离重置仅剩 1~2 天甚至数小时）但已用比例较高（如 88%）的账号
// 长期无法获得请求分配，剩余额度来不及用完就被重置刷新，造成严重的配额浪费；而距离重置时间尚长（如还有 5 天以上）已用 60% 的账号
// 额度充裕却被持续消耗。
//
// 因此将用量百分比 (0-100) 与剩余重置时间百分比 (0-100) 结合为一个综合数值：
//
//	resetRemainingPercent = clamp(resetRemainingSeconds / 7d, 0, 1) * 100
//	score = usedPercent + resetRemainingPercent
//
// 数学含义：
// score - 100 = 100 * (剩余时间比例 - 剩余额度比例)。
// - 当账号临近重置（剩余时间比例迅速缩小），即便已用百分比稍高，其综合评分也会显著降低（优先级提升），
//   促使系统优先消耗即将过期的剩余额度，避免重置浪费；
// - 当重置时间相近时，平滑退化为用量百分比比较（用量更小优先）；
// - 当用量百分比相近时，平滑退化为剩余重置时间比较（更靠近重置优先）；
// - 当无重置时间记录时，剩余时间视为满周期 (100%)，直接退化为按用量排序。
func calculateOAuth7dSchedulingScore(usedPercent float64, resetRemainingSeconds int64) float64 {
	if math.IsNaN(usedPercent) || math.IsInf(usedPercent, 0) || usedPercent < 0 {
		usedPercent = 0
	}
	var resetRemainingPercent float64
	switch {
	case resetRemainingSeconds <= 0:
		resetRemainingPercent = 0.0
	case resetRemainingSeconds >= oauth7dWindowSeconds:
		resetRemainingPercent = 100.0
	default:
		resetRemainingPercent = (float64(resetRemainingSeconds) / float64(oauth7dWindowSeconds)) * 100.0
	}
	return usedPercent + resetRemainingPercent
}

// compareOAuth7dSchedulingUsage 比较两个 OAuth 账号的 7 天调度指标（OpenAI / Anthropic 通用）：
// 1. 额度耗尽保护：未耗尽（<100%）的账号绝对优先于已耗尽（>=100%）的账号；若均耗尽则离重置更近的优先；
// 2. 结合用量百分比与距离重置时间计算综合评分（越小越优先）；
// 3. 综合评分极度接近时，距离重置时间更近的优先；
// 4. 重置时间也相同时，用量更小的优先。
func compareOAuth7dSchedulingUsage(leftResetRemainingSeconds, rightResetRemainingSeconds int64, leftUsedPercent, rightUsedPercent float64) (bool, bool) {
	// 1. 额度耗尽保护
	leftExhausted := leftUsedPercent >= 100.0
	rightExhausted := rightUsedPercent >= 100.0
	if leftExhausted != rightExhausted {
		return !leftExhausted, true
	}
	if leftExhausted && rightExhausted {
		if leftResetRemainingSeconds != rightResetRemainingSeconds {
			return leftResetRemainingSeconds < rightResetRemainingSeconds, true
		}
		return false, false
	}

	// 2. 结合用量与剩余时间计算综合得分（越小越优先）
	leftScore := calculateOAuth7dSchedulingScore(leftUsedPercent, leftResetRemainingSeconds)
	rightScore := calculateOAuth7dSchedulingScore(rightUsedPercent, rightResetRemainingSeconds)

	const scoreEpsilon = 1e-4
	if math.Abs(leftScore-rightScore) > scoreEpsilon {
		return leftScore < rightScore, true
	}

	// 3. 平局决胜：距离重置时间更近的优先
	if leftResetRemainingSeconds != rightResetRemainingSeconds {
		return leftResetRemainingSeconds < rightResetRemainingSeconds, true
	}

	// 4. 若重置时间也相同，则用量更小的优先
	if math.Abs(leftUsedPercent-rightUsedPercent) > scoreEpsilon {
		return leftUsedPercent < rightUsedPercent, true
	}

	return false, false
}
