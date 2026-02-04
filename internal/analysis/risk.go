package analysis

import (
	"fmt"
	"math"
)

// 강화 확률표 (공식)
var enhanceProbabilities = map[int]struct {
	Success float64
	Hold    float64
	Destroy float64
}{
	1:  {90, 10, 0},
	2:  {85, 15, 0},
	3:  {80, 20, 0},
	4:  {75, 25, 0},
	5:  {60, 35, 5},
	6:  {55, 35, 10},
	7:  {50, 35, 15},
	8:  {45, 35, 20},
	9:  {35, 40, 25},
	10: {25, 45, 30},
	11: {15, 50, 35},
	12: {10, 50, 40},
	13: {7, 53, 40},
	14: {5, 55, 40},
	15: {3, 57, 40},
}

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

// calculateSuccessProb 목표 도달 확률 계산
func calculateSuccessProb(currentLevel, targetLevel int) float64 {
	if currentLevel >= targetLevel {
		return 100.0
	}

	prob := 1.0
	for level := currentLevel; level < targetLevel; level++ {
		if p, ok := enhanceProbabilities[level]; ok {
			// 각 레벨 강화 성공 확률을 곱함 (파괴 없이 성공할 확률)
			levelProb := p.Success / 100.0
			// 파괴 고려: 평균적으로 몇 번 시도해야 성공하는지
			if p.Destroy > 0 {
				// 파괴 시 다시 시작해야 하므로 복잡한 계산 필요
				// 간이 계산: 파괴 확률만큼 확률 감소
				levelProb *= (1 - p.Destroy/100.0*0.5) // 파괴 시 절반 확률 감소로 근사
			}
			prob *= levelProb
		}
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
func calculateExpectedCost(currentLevel, targetLevel int) int {
	// 강화 비용 테이블 (예시)
	enhanceCost := map[int]int{
		1: 100, 2: 200, 3: 400, 4: 800, 5: 1500,
		6: 3000, 7: 5000, 8: 8000, 9: 15000, 10: 25000,
		11: 40000, 12: 60000, 13: 80000, 14: 100000, 15: 150000,
	}

	totalCost := 0
	for level := currentLevel; level < targetLevel; level++ {
		cost, ok := enhanceCost[level]
		if !ok {
			cost = 50000 // 기본값
		}

		prob, ok := enhanceProbabilities[level]
		if !ok {
			continue
		}

		// 평균 시도 횟수 = 1 / 성공확률
		avgTrials := 1.0 / (prob.Success / 100.0)
		totalCost += int(float64(cost) * avgTrials)
	}

	return totalCost
}

// calculateExpectedTrials 예상 시도 횟수
func calculateExpectedTrials(currentLevel, targetLevel int) int {
	trials := 0
	for level := currentLevel; level < targetLevel; level++ {
		if prob, ok := enhanceProbabilities[level]; ok {
			avgTrials := 1.0 / (prob.Success / 100.0)
			trials += int(math.Ceil(avgTrials))
		}
	}
	return trials
}

// calculateKellyRatio 켈리 기준 최적 배팅 비율
// Kelly = (bp - q) / b
// b = 승리 시 수익률, p = 승리 확률, q = 패배 확률
func calculateKellyRatio(currentLevel, targetLevel int) float64 {
	if currentLevel >= targetLevel {
		return 0
	}

	// 다음 강화의 켈리 비율 계산
	prob, ok := enhanceProbabilities[currentLevel]
	if !ok {
		return 0.05 // 기본 보수적 비율
	}

	p := prob.Success / 100.0
	q := 1 - p

	// 간이 수익률 계산 (성공 시 레벨업 가치)
	b := 1.5 // 대략적 수익률

	kelly := (b*p - q) / b
	// 보수적으로 절반 켈리 사용
	return math.Max(0, math.Min(0.25, kelly*0.5))
}

// calculateExpectedDrawdown 예상 최대 낙폭
func calculateExpectedDrawdown(currentLevel, targetLevel int) float64 {
	// 파괴 확률 기반 예상 낙폭
	maxDestroy := 0.0
	for level := currentLevel; level < targetLevel; level++ {
		if prob, ok := enhanceProbabilities[level]; ok {
			if prob.Destroy > maxDestroy {
				maxDestroy = prob.Destroy
			}
		}
	}

	// 파괴 확률이 높을수록 낙폭 증가
	return math.Min(80, maxDestroy*1.5+10)
}

// calculateExpectedGold 기대 최종 골드
func calculateExpectedGold(currentLevel, targetLevel, currentGold int) int {
	// 판매가 테이블 (예시)
	sellPrice := map[int]int{
		5: 5000, 6: 10000, 7: 20000, 8: 40000, 9: 80000,
		10: 150000, 11: 300000, 12: 600000, 13: 1200000, 14: 2500000, 15: 5000000,
	}

	successProb := calculateSuccessProb(currentLevel, targetLevel)
	expectedCost := calculateExpectedCost(currentLevel, targetLevel)

	targetPrice, ok := sellPrice[targetLevel]
	if !ok {
		targetPrice = 100000 // 기본값
	}

	// 기대값 = (성공확률 * 판매가) - 예상비용
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
		formatGold(r.CurrentGold),
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

// formatGold 골드를 콤마 표기로 포맷 (game.FormatGold와 동일, 순환참조 방지용)
func formatGold(gold int) string {
	if gold == 0 {
		return "0"
	}
	s := ""
	negative := false
	n := gold
	if n < 0 {
		negative = true
		n = -n
	}
	for n > 0 {
		if s != "" && len(s)%4 == 3 {
			s = "," + s
		}
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if negative {
		s = "-" + s
	}
	return s
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
