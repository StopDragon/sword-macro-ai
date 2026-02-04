package analysis

import (
	"fmt"
	"math"

	"github.com/StopDragon/sword-macro-ai/internal/game"
)

// RiskAnalysis 리스크 분석 결과
type RiskAnalysis struct {
	CurrentLevel int `json:"current_level"`
	CurrentGold  int `json:"current_gold"`
	TargetLevel  int `json:"target_level"`

	// 확률 분석
	SuccessProb    float64 `json:"success_prob"`     // 목표 도달 확률 (%)
	RuinProb       float64 `json:"ruin_prob"`        // 파산 확률 (%)
	ExpectedGold   int     `json:"expected_gold"`    // 기대 최종 골드
	ExpectedTrials int     `json:"expected_trials"`  // 예상 시도 횟수

	// 켈리 기준
	KellyBetRatio float64 `json:"kelly_bet_ratio"` // 최적 배팅 비율 (0-1)
	MaxDrawdown   float64 `json:"max_drawdown"`    // 예상 최대 낙폭 (%)

	// 추천
	Recommendation string `json:"recommendation"`     // "enhance", "sell", "wait", "battle"
	Warning        string `json:"warning,omitempty"`  // 경고 메시지
	Confidence     string `json:"confidence"`         // "low", "medium", "high"
}

// CalcRisk 리스크 계산
func CalcRisk(currentLevel, currentGold, targetLevel int) *RiskAnalysis {
	analysis := &RiskAnalysis{
		CurrentLevel: currentLevel,
		CurrentGold:  currentGold,
		TargetLevel:  targetLevel,
	}

	// 목표까지 도달 확률 계산
	analysis.SuccessProb = calculateSuccessProb(currentLevel, targetLevel)

	// 파산 확률 계산 (간이 버전)
	analysis.RuinProb = calculateRuinProb(currentLevel, targetLevel, currentGold)

	// 예상 시도 횟수
	analysis.ExpectedTrials = calculateExpectedTrials(currentLevel, targetLevel)

	// 켈리 기준 계산
	analysis.KellyBetRatio = calculateKellyRatio(currentLevel, targetLevel)

	// 예상 최대 낙폭
	analysis.MaxDrawdown = calculateExpectedDrawdown(currentLevel, targetLevel)

	// 기대 골드 계산
	analysis.ExpectedGold = calculateExpectedGold(currentLevel, targetLevel, currentGold)

	// 추천 및 경고 생성
	analysis.generateRecommendation()

	return analysis
}

// calculateSuccessProb 목표 도달 확률 계산 (API 데이터 기반)
func calculateSuccessProb(currentLevel, targetLevel int) float64 {
	if currentLevel >= targetLevel {
		return 100.0
	}

	prob := 1.0
	for level := currentLevel; level < targetLevel; level++ {
		rate := game.GetEnhanceRate(level)
		if rate == nil {
			continue
		}
		levelProb := rate.SuccessRate / 100.0
		if rate.DestroyRate > 0 {
			levelProb *= (1 - rate.DestroyRate/100.0*0.5)
		}
		prob *= levelProb
	}

	return math.Max(0, math.Min(100, prob*100))
}

// calculateRuinProb 파산 확률 계산
func calculateRuinProb(currentLevel, targetLevel, currentGold int) float64 {
	// 간이 계산: 목표까지 예상 소요 골드 vs 현재 골드
	expectedCost := calculateExpectedCost(currentLevel, targetLevel)
	if currentGold <= 0 {
		return 100.0
	}

	// 파산 확률 근사: 비용이 자본의 몇 배인지
	ratio := float64(expectedCost) / float64(currentGold)
	if ratio <= 0.5 {
		return 5.0 // 충분한 자본
	} else if ratio <= 1.0 {
		return 15.0 + ratio*20
	} else if ratio <= 2.0 {
		return 35.0 + (ratio-1)*25
	} else {
		return math.Min(95, 60+ratio*10)
	}
}

// calculateExpectedCost 예상 소요 골드 (강화 비용)
// 강화 비용 = 해당 레벨 검 가격의 약 10% (간이 추정)
func calculateExpectedCost(currentLevel, targetLevel int) int {
	totalCost := 0
	for level := currentLevel; level < targetLevel; level++ {
		// 강화 비용은 해당 레벨 검 평균 가격의 10%로 추정
		price := game.GetSwordPrice(level)
		cost := 100 // 기본값
		if price != nil {
			cost = price.AvgPrice / 10
			if cost < 100 {
				cost = 100
			}
		}

		rate := game.GetEnhanceRate(level)
		if rate == nil || rate.SuccessRate <= 0 {
			continue
		}

		avgTrials := 1.0 / (rate.SuccessRate / 100.0)
		totalCost += int(float64(cost) * avgTrials)
	}

	return totalCost
}

// calculateExpectedTrials 예상 시도 횟수 (API 데이터 기반)
func calculateExpectedTrials(currentLevel, targetLevel int) int {
	trials := 0
	for level := currentLevel; level < targetLevel; level++ {
		rate := game.GetEnhanceRate(level)
		if rate != nil && rate.SuccessRate > 0 {
			avgTrials := 1.0 / (rate.SuccessRate / 100.0)
			trials += int(math.Ceil(avgTrials))
		}
	}
	return trials
}

// calculateKellyRatio 켈리 기준 최적 배팅 비율 (API 데이터 기반)
// Kelly = (bp - q) / b
func calculateKellyRatio(currentLevel, targetLevel int) float64 {
	if currentLevel >= targetLevel {
		return 0
	}

	rate := game.GetEnhanceRate(currentLevel)
	if rate == nil {
		return 0.05
	}

	p := rate.SuccessRate / 100.0
	q := 1 - p
	b := 1.5

	kelly := (b*p - q) / b
	return math.Max(0, math.Min(0.25, kelly*0.5))
}

// calculateExpectedDrawdown 예상 최대 낙폭 (API 데이터 기반)
func calculateExpectedDrawdown(currentLevel, targetLevel int) float64 {
	maxDestroy := 0.0
	for level := currentLevel; level < targetLevel; level++ {
		rate := game.GetEnhanceRate(level)
		if rate != nil && rate.DestroyRate > maxDestroy {
			maxDestroy = rate.DestroyRate
		}
	}

	return math.Min(80, maxDestroy*1.5+10)
}

// calculateExpectedGold 기대 최종 골드 (API 데이터 기반)
func calculateExpectedGold(currentLevel, targetLevel, currentGold int) int {
	successProb := calculateSuccessProb(currentLevel, targetLevel)
	expectedCost := calculateExpectedCost(currentLevel, targetLevel)

	targetPrice := 100000 // 기본값
	price := game.GetSwordPrice(targetLevel)
	if price != nil {
		targetPrice = price.AvgPrice
	}

	expectedReturn := int(float64(targetPrice)*(successProb/100)) - expectedCost
	return currentGold + expectedReturn
}

// generateRecommendation 추천 및 경고 생성
func (r *RiskAnalysis) generateRecommendation() {
	// 신뢰도 결정
	if r.CurrentGold < 10000 {
		r.Confidence = "low"
	} else if r.CurrentGold < 100000 {
		r.Confidence = "medium"
	} else {
		r.Confidence = "high"
	}

	// 파산 위험이 높으면 경고
	if r.RuinProb > 50 {
		r.Warning = fmt.Sprintf("파산 위험 %.0f%% - 목표 레벨 하향 권장", r.RuinProb)
		r.Recommendation = "sell"
	} else if r.RuinProb > 30 {
		r.Warning = fmt.Sprintf("파산 위험 %.0f%% - 주의 필요", r.RuinProb)
		r.Recommendation = "wait"
	} else if r.SuccessProb < 10 {
		r.Warning = "성공 확률이 매우 낮음"
		r.Recommendation = "sell"
	} else {
		r.Recommendation = "enhance"
	}

	// 자본이 충분하면 강화 권장
	if r.KellyBetRatio > 0.1 && r.RuinProb < 20 {
		r.Recommendation = "enhance"
	}
}

// FormatRiskAnalysis 리스크 분석 결과 포맷팅
func FormatRiskAnalysis(r *RiskAnalysis) string {
	result := fmt.Sprintf(`
⚠️ 리스크 분석 (현재: +%d, %s골드)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
목표 +%d 도달: %.1f%% 확률
파산 위험: %.0f%%
예상 소요: %d회 시도

📊 켈리 기준 배팅: 골드의 %.0f%%
📉 예상 최대 낙폭: %.0f%%

💡 추천: %s`,
		r.CurrentLevel,
		game.FormatGold(r.CurrentGold),
		r.TargetLevel,
		r.SuccessProb,
		r.RuinProb,
		r.ExpectedTrials,
		r.KellyBetRatio*100,
		r.MaxDrawdown,
		translateRecommendation(r.Recommendation),
	)

	if r.Warning != "" {
		result += fmt.Sprintf("\n⚠️ 경고: %s", r.Warning)
	}

	return result
}

func translateRecommendation(rec string) string {
	switch rec {
	case "enhance":
		return "강화 진행"
	case "sell":
		return "판매 권장"
	case "wait":
		return "대기 권장"
	case "battle":
		return "배틀 추천"
	default:
		return rec
	}
}
