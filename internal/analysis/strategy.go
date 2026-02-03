package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// StrategyProfile 전략 프로필
type StrategyProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	// 강화 전략
	TargetLevel  int   `json:"target_level"`   // 목표 레벨
	SellLevels   []int `json:"sell_levels"`    // 판매 기준 레벨들
	StopLossGold int   `json:"stop_loss_gold"` // 손절 기준 골드

	// 배틀 전략
	EnableBattle  bool `json:"enable_battle"`   // 배틀 활성화
	MaxUpsetDiff  int  `json:"max_upset_diff"`  // 최대 역배 레벨차
	MinBattleGold int  `json:"min_battle_gold"` // 배틀 최소 골드

	// 리스크 관리
	MaxBetRatio float64 `json:"max_bet_ratio"` // 최대 배팅 비율 (0-1)
	MaxRuinProb float64 `json:"max_ruin_prob"` // 허용 파산 확률 (0-1)

	// 자동화
	AutoSell   bool `json:"auto_sell"`   // 자동 판매
	AutoBattle bool `json:"auto_battle"` // 자동 배틀

	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

// StrategyManager 전략 관리자
type StrategyManager struct {
	strategies    []StrategyProfile
	currentIndex  int
	configPath    string
}

// 기본 제공 전략들
var defaultStrategies = []StrategyProfile{
	{
		Name:         "안전한 10강러",
		Description:  "저위험 안정적 수익",
		TargetLevel:  10,
		SellLevels:   []int{10},
		MaxUpsetDiff: 1,
		MaxBetRatio:  0.05,
		MaxRuinProb:  0.15,
		EnableBattle: false,
		AutoSell:     true,
	},
	{
		Name:         "공격적 12강러",
		Description:  "고위험 고수익",
		TargetLevel:  12,
		SellLevels:   []int{12, 11},
		MaxUpsetDiff: 2,
		MaxBetRatio:  0.15,
		MaxRuinProb:  0.35,
		EnableBattle: true,
		AutoSell:     true,
	},
	{
		Name:         "역배 전문가",
		Description:  "배틀 중심 플레이",
		TargetLevel:  8,
		SellLevels:   []int{8, 9, 10},
		MaxUpsetDiff: 3,
		MaxBetRatio:  0.10,
		MaxRuinProb:  0.25,
		EnableBattle: true,
		AutoBattle:   true,
	},
	{
		Name:         "히든 헌터",
		Description:  "히든 검 파밍 전문",
		TargetLevel:  5,
		SellLevels:   []int{5, 6, 7},
		MaxUpsetDiff: 0,
		MaxBetRatio:  0.03,
		MaxRuinProb:  0.10,
		EnableBattle: false,
	},
}

// NewStrategyManager 새 전략 관리자 생성
func NewStrategyManager() *StrategyManager {
	sm := &StrategyManager{
		strategies:   make([]StrategyProfile, len(defaultStrategies)),
		currentIndex: 0,
	}

	// 기본 전략 복사
	copy(sm.strategies, defaultStrategies)

	// 설정 파일 경로
	exe, err := os.Executable()
	if err == nil {
		sm.configPath = filepath.Join(filepath.Dir(exe), "strategies.json")
	} else {
		sm.configPath = "strategies.json"
	}

	// 저장된 전략 로드
	sm.loadStrategies()

	return sm
}

// GetStrategies 모든 전략 반환
func (sm *StrategyManager) GetStrategies() []StrategyProfile {
	return sm.strategies
}

// GetCurrentStrategy 현재 전략 반환
func (sm *StrategyManager) GetCurrentStrategy() *StrategyProfile {
	if sm.currentIndex < 0 || sm.currentIndex >= len(sm.strategies) {
		return nil
	}
	return &sm.strategies[sm.currentIndex]
}

// SetCurrentStrategy 현재 전략 설정
func (sm *StrategyManager) SetCurrentStrategy(index int) bool {
	if index < 0 || index >= len(sm.strategies) {
		return false
	}
	sm.currentIndex = index
	sm.strategies[index].LastUsed = time.Now()
	sm.saveStrategies()
	return true
}

// AddCustomStrategy 커스텀 전략 추가
func (sm *StrategyManager) AddCustomStrategy(strategy StrategyProfile) {
	strategy.CreatedAt = time.Now()
	sm.strategies = append(sm.strategies, strategy)
	sm.saveStrategies()
}

// ShouldSell 현재 레벨에서 판매해야 하는지 확인
func (sm *StrategyManager) ShouldSell(currentLevel int) bool {
	strategy := sm.GetCurrentStrategy()
	if strategy == nil {
		return false
	}

	for _, level := range strategy.SellLevels {
		if currentLevel >= level {
			return true
		}
	}
	return false
}

// ShouldBattle 배틀 해도 되는지 확인
func (sm *StrategyManager) ShouldBattle(levelDiff, currentGold int) bool {
	strategy := sm.GetCurrentStrategy()
	if strategy == nil || !strategy.EnableBattle {
		return false
	}

	if levelDiff > strategy.MaxUpsetDiff {
		return false
	}

	if currentGold < strategy.MinBattleGold {
		return false
	}

	return true
}

// CheckRiskLimits 리스크 한도 확인
func (sm *StrategyManager) CheckRiskLimits(risk *RiskAnalysis) (bool, string) {
	strategy := sm.GetCurrentStrategy()
	if strategy == nil {
		return true, ""
	}

	// 파산 확률 체크
	if risk.RuinProb/100 > strategy.MaxRuinProb {
		return false, "파산 확률이 전략 한도를 초과합니다"
	}

	// 배팅 비율 체크
	if risk.KellyBetRatio > strategy.MaxBetRatio {
		return false, "권장 배팅이 전략 한도를 초과합니다"
	}

	return true, ""
}

// loadStrategies 전략 파일 로드
func (sm *StrategyManager) loadStrategies() {
	data, err := os.ReadFile(sm.configPath)
	if err != nil {
		return // 파일 없으면 기본 전략 사용
	}

	var saved struct {
		Strategies   []StrategyProfile `json:"strategies"`
		CurrentIndex int               `json:"current_index"`
	}

	if err := json.Unmarshal(data, &saved); err != nil {
		return
	}

	// 기본 전략과 저장된 전략 병합
	if len(saved.Strategies) > len(defaultStrategies) {
		sm.strategies = saved.Strategies
	}
	sm.currentIndex = saved.CurrentIndex
}

// saveStrategies 전략 파일 저장
func (sm *StrategyManager) saveStrategies() {
	saved := struct {
		Strategies   []StrategyProfile `json:"strategies"`
		CurrentIndex int               `json:"current_index"`
	}{
		Strategies:   sm.strategies,
		CurrentIndex: sm.currentIndex,
	}

	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(sm.configPath, data, 0644)
}

// FormatStrategy 전략 포맷팅
func FormatStrategy(s *StrategyProfile) string {
	if s == nil {
		return "전략 없음"
	}

	battleStr := "비활성"
	if s.EnableBattle {
		battleStr = "활성"
		if s.AutoBattle {
			battleStr = "자동"
		}
	}

	return `
전략: ` + s.Name + `
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📝 ` + s.Description + `
🎯 목표 레벨: +` + string(rune('0'+s.TargetLevel)) + `
💰 판매 기준: +` + formatLevels(s.SellLevels) + `
⚔️ 배틀: ` + battleStr + `
📊 최대 파산 허용: ` + formatPercent(s.MaxRuinProb) + `
`
}

func formatLevels(levels []int) string {
	if len(levels) == 0 {
		return "없음"
	}
	result := ""
	for i, level := range levels {
		if i > 0 {
			result += ", "
		}
		result += string(rune('0' + level/10))
		result += string(rune('0' + level%10))
	}
	return result
}

func formatPercent(ratio float64) string {
	return string(rune('0'+int(ratio*100)/10)) + string(rune('0'+int(ratio*100)%10)) + "%"
}
