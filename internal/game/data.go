package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// 캐시 (매 호출마다 HTTP 요청 방지)
var (
	gameDataCache     *GameData
	gameDataCacheTime time.Time
	gameDataMu        sync.Mutex
	gameDataTTL       = 5 * time.Minute

	optimalSellCache     *OptimalSellData
	optimalSellCacheTime time.Time
	optimalSellMu        sync.Mutex
	optimalSellTTL       = 10 * time.Minute
)

const (
	gameDataEndpoint    = "https://sword-ai.stopdragon.kr/api/game-data"
	optimalSellEndpoint = "https://sword-ai.stopdragon.kr/api/strategy/optimal-sell-point"
)

// EnhanceRate 강화 확률 데이터 (레벨별)
type EnhanceRate struct {
	Level       int     `json:"level"`
	SuccessRate float64 `json:"success_rate"`
	KeepRate    float64 `json:"keep_rate"`
	DestroyRate float64 `json:"destroy_rate"`
}

// SwordPrice 검 판매가 데이터 (레벨별)
type SwordPrice struct {
	Level    int `json:"level"`
	MinPrice int `json:"min_price"`
	MaxPrice int `json:"max_price"`
	AvgPrice int `json:"avg_price"`
}

// BattleReward 배틀 보상 데이터
type BattleReward struct {
	LevelDiff int     `json:"level_diff"`
	WinRate   float64 `json:"win_rate"`
	MinReward int     `json:"min_reward"`
	MaxReward int     `json:"max_reward"`
	AvgReward int     `json:"avg_reward"`
}

// GameData 서버에서 가져오는 게임 데이터
type GameData struct {
	EnhanceRates  []EnhanceRate  `json:"enhance_rates"`
	SwordPrices   []SwordPrice   `json:"sword_prices"`
	BattleRewards []BattleReward `json:"battle_rewards"`
	UpdatedAt     string         `json:"updated_at"`
}

// LevelEfficiency 레벨별 효율성 데이터
type LevelEfficiency struct {
	Level              int     `json:"level"`
	AvgPrice           int     `json:"avg_price"`
	ExpectedTrials     float64 `json:"expected_trials"`
	ExpectedTimeSecond float64 `json:"expected_time_second"`
	SuccessProb        float64 `json:"success_prob"`
	GoldPerMinute      float64 `json:"gold_per_minute"`
	Recommendation     string  `json:"recommendation"`
}

// TypeOptimal 타입별 최적 판매 데이터
type TypeOptimal struct {
	Type           string  `json:"type"`
	OptimalLevel   int     `json:"optimal_level"`
	OptimalGPM     float64 `json:"optimal_gpm"`
	SampleSize     int     `json:"sample_size"`
	EnhanceSamples int     `json:"enhance_samples"`
	IsDefault      bool    `json:"is_default"`
}

// OptimalSellData 최적 판매 시점 데이터
type OptimalSellData struct {
	OptimalLevel      int                    `json:"optimal_level"`
	OptimalGPM        float64                `json:"optimal_gpm"`
	LevelEfficiencies []LevelEfficiency      `json:"level_efficiencies"`
	ByType            map[string]TypeOptimal `json:"by_type"` // 타입별 최적 레벨
	Note              string                 `json:"note"`
}


// FetchGameData 서버에서 게임 데이터 가져오기 (TTL 캐시 적용)
func FetchGameData() (*GameData, error) {
	gameDataMu.Lock()
	defer gameDataMu.Unlock()

	if gameDataCache != nil && time.Since(gameDataCacheTime) < gameDataTTL {
		return gameDataCache, nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(gameDataEndpoint)
	if err != nil {
		if gameDataCache != nil {
			return gameDataCache, nil // 실패 시 이전 캐시 반환
		}
		return nil, fmt.Errorf("서버 연결 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if gameDataCache != nil {
			return gameDataCache, nil
		}
		return nil, fmt.Errorf("서버 오류: %d", resp.StatusCode)
	}

	var data GameData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		if gameDataCache != nil {
			return gameDataCache, nil
		}
		return nil, fmt.Errorf("데이터 파싱 실패: %v", err)
	}

	gameDataCache = &data
	gameDataCacheTime = time.Now()
	return &data, nil
}

// InitGameData 게임 데이터 초기화 (앱 시작 시 호출)
func InitGameData() error {
	_, err := FetchGameData()
	return err
}

// GetEnhanceRate 특정 레벨의 강화 확률 조회
func GetEnhanceRate(level int) *EnhanceRate {
	data, err := FetchGameData()
	if err != nil || data == nil {
		return nil
	}

	if level < 0 || level >= len(data.EnhanceRates) {
		return nil
	}
	return &data.EnhanceRates[level]
}

// GetSwordPrice 특정 레벨의 검 판매가 조회
func GetSwordPrice(level int) *SwordPrice {
	data, err := FetchGameData()
	if err != nil || data == nil {
		return nil
	}

	if level < 0 || level >= len(data.SwordPrices) {
		return nil
	}
	return &data.SwordPrices[level]
}

// GetBattleReward 특정 레벨 차이의 배틀 보상 조회
func GetBattleReward(levelDiff int) *BattleReward {
	data, err := FetchGameData()
	if err != nil || data == nil {
		return nil
	}

	for i := range data.BattleRewards {
		if data.BattleRewards[i].LevelDiff == levelDiff {
			return &data.BattleRewards[i]
		}
	}
	return nil
}

// GetAllEnhanceRates 모든 강화 확률 조회
func GetAllEnhanceRates() []EnhanceRate {
	data, err := FetchGameData()
	if err != nil || data == nil {
		return nil
	}
	return data.EnhanceRates
}

// GetAllSwordPrices 모든 검 판매가 조회
func GetAllSwordPrices() []SwordPrice {
	data, err := FetchGameData()
	if err != nil || data == nil {
		return nil
	}
	return data.SwordPrices
}

// GetAllBattleRewards 모든 배틀 보상 조회
func GetAllBattleRewards() []BattleReward {
	data, err := FetchGameData()
	if err != nil || data == nil {
		return nil
	}
	return data.BattleRewards
}

// CalcUpsetExpectedValue 역배 기대값 계산
func CalcUpsetExpectedValue(myLevel, targetLevel, betAmount int) (expectedValue float64, winRate float64, avgReward int) {
	levelDiff := targetLevel - myLevel
	reward := GetBattleReward(levelDiff)
	if reward == nil {
		return 0, 0, 0
	}

	winRate = reward.WinRate / 100.0
	loseRate := 1.0 - winRate
	avgReward = reward.AvgReward

	// 기대값 = (승률 × 획득 골드) - (패율 × 손실 골드)
	expectedValue = (winRate * float64(avgReward)) - (loseRate * float64(betAmount))
	return expectedValue, reward.WinRate, avgReward
}

// CalcEnhanceSuccessChance 목표 레벨까지 강화 성공 확률 계산
func CalcEnhanceSuccessChance(currentLevel, targetLevel int) float64 {
	if currentLevel >= targetLevel {
		return 100.0
	}

	rates := GetAllEnhanceRates()
	if rates == nil {
		return 0.0
	}

	chance := 1.0
	for level := currentLevel; level < targetLevel && level < len(rates); level++ {
		chance *= rates[level].SuccessRate / 100.0
	}

	return chance * 100.0
}

// CalcExpectedTrials 목표 레벨까지 평균 시도 횟수 계산
func CalcExpectedTrials(currentLevel, targetLevel int) float64 {
	if currentLevel >= targetLevel {
		return 0
	}

	rates := GetAllEnhanceRates()
	if rates == nil {
		return 0
	}

	totalTrials := 0.0
	for level := currentLevel; level < targetLevel && level < len(rates); level++ {
		if rates[level].SuccessRate > 0 {
			totalTrials += 100.0 / rates[level].SuccessRate
		}
	}

	return totalTrials
}

// CalcOptimalSellLevel 골드 채굴 최적 판매 레벨 계산
// 강화 확률과 판매가를 고려하여 기대 수익이 가장 높은 레벨 반환
// currentGold: 현재 보유 골드 (비용 고려용)
func CalcOptimalSellLevel(currentGold int) int {
	rates := GetAllEnhanceRates()
	prices := GetAllSwordPrices()

	if rates == nil || prices == nil {
		return 10 // 기본값
	}

	// 각 목표 레벨별 기대 수익 계산
	bestLevel := 10
	bestExpectedProfit := 0.0

	// 레벨 5~15 범위에서 최적 레벨 탐색
	for targetLevel := 5; targetLevel <= 15 && targetLevel < len(prices); targetLevel++ {
		// 0강에서 targetLevel까지 성공 확률
		successChance := 1.0
		for level := 0; level < targetLevel && level < len(rates); level++ {
			successChance *= rates[level].SuccessRate / 100.0
		}

		// 목표 레벨의 판매가
		price := prices[targetLevel].AvgPrice

		// 기대 수익 = 성공 확률 × 판매가
		expectedProfit := successChance * float64(price)

		// 레벨이 높아질수록 시간 비용이 증가하므로 페널티 적용
		// (레벨당 약 5% 시간 비용 추가로 가정)
		timePenalty := 1.0 - float64(targetLevel-5)*0.02

		adjustedProfit := expectedProfit * timePenalty

		if adjustedProfit > bestExpectedProfit {
			bestExpectedProfit = adjustedProfit
			bestLevel = targetLevel
		}
	}

	return bestLevel
}

// FetchOptimalSellData 서버에서 최적 판매 시점 데이터 가져오기 (TTL 캐시 적용)
func FetchOptimalSellData() (*OptimalSellData, error) {
	optimalSellMu.Lock()
	defer optimalSellMu.Unlock()

	if optimalSellCache != nil && time.Since(optimalSellCacheTime) < optimalSellTTL {
		return optimalSellCache, nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(optimalSellEndpoint)
	if err != nil {
		if optimalSellCache != nil {
			return optimalSellCache, nil
		}
		return nil, fmt.Errorf("서버 연결 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if optimalSellCache != nil {
			return optimalSellCache, nil
		}
		return nil, fmt.Errorf("서버 오류: %d", resp.StatusCode)
	}

	var data OptimalSellData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		if optimalSellCache != nil {
			return optimalSellCache, nil
		}
		return nil, fmt.Errorf("데이터 파싱 실패: %v", err)
	}

	optimalSellCache = &data
	optimalSellCacheTime = time.Now()
	return &data, nil
}

// GetOptimalSellLevel 서버 통계 기반 최적 판매 레벨 조회
// 서버에서 커뮤니티 데이터 기반 추천값이 있으면 사용, 없으면 로컬 계산
func GetOptimalSellLevel(currentGold int) (level int, source string) {
	// 서버 API 호출 시도
	data, err := FetchOptimalSellData()
	if err == nil && data != nil && data.OptimalLevel > 0 {
		return data.OptimalLevel, fmt.Sprintf("서버(%.0f G/분)", data.OptimalGPM)
	}

	// 서버 실패 시 로컬 계산 사용
	level = CalcOptimalSellLevel(currentGold)
	source = "계산값"

	return level, source
}

// GetLevelEfficiency 특정 레벨의 효율성 데이터 조회
func GetLevelEfficiency(level int) *LevelEfficiency {
	data, err := FetchOptimalSellData()
	if err != nil || data == nil {
		return nil
	}

	for i := range data.LevelEfficiencies {
		if data.LevelEfficiencies[i].Level == level {
			return &data.LevelEfficiencies[i]
		}
	}
	return nil
}

// GetAllLevelEfficiencies 모든 레벨 효율성 데이터 조회
func GetAllLevelEfficiencies() []LevelEfficiency {
	data, err := FetchOptimalSellData()
	if err != nil || data == nil {
		return nil
	}
	return data.LevelEfficiencies
}

// GetOptimalLevelByType 타입별 최적 판매 레벨 조회
// itemType: "normal", "special", "trash"
// 반환: (최적 레벨, 기본값 사용 여부)
func GetOptimalLevelByType(itemType string) (int, bool) {
	data, err := FetchOptimalSellData()
	if err != nil || data == nil || data.ByType == nil {
		return getDefaultOptimalLevel(itemType), true
	}

	if typeData, ok := data.ByType[itemType]; ok {
		return typeData.OptimalLevel, typeData.IsDefault
	}

	return getDefaultOptimalLevel(itemType), true
}

// GetOptimalLevelsByType 모든 타입의 최적 판매 레벨 조회
// 반환: map[타입]최적레벨 (예: {"normal": 10, "special": 12})
func GetOptimalLevelsByType() map[string]int {
	result := map[string]int{
		"normal":  10,
		"special": 10,
		"trash":   0,
	}

	data, err := FetchOptimalSellData()
	if err != nil || data == nil || data.ByType == nil {
		return result
	}

	for itemType, typeData := range data.ByType {
		if typeData.OptimalLevel > 0 {
			result[itemType] = typeData.OptimalLevel
		}
	}

	return result
}

// getDefaultOptimalLevel 타입별 기본 최적 레벨
func getDefaultOptimalLevel(itemType string) int {
	switch itemType {
	case "trash":
		return 0 // 쓰레기는 강화 안함
	case "special":
		return 10 // 특수 아이템 기본값
	default:
		return 10 // 일반 아이템 기본값
	}
}

// FormatGold 골드를 정확한 콤마 표기로 포맷 (예: 184,331,258)
func FormatGold(gold int) string {
	return formatInt(gold)
}

func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return formatInt(int(f))
	}
	return formatFloatWithPrecision(f, 1)
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	negative := false
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

func formatFloatWithPrecision(f float64, precision int) string {
	intPart := int(f)
	fracPart := f - float64(intPart)

	for i := 0; i < precision; i++ {
		fracPart *= 10
	}

	fracInt := int(fracPart + 0.5)

	result := formatInt(intPart) + "."
	fracStr := ""
	for fracInt > 0 || len(fracStr) < precision {
		fracStr = string(rune('0'+fracInt%10)) + fracStr
		fracInt /= 10
		if len(fracStr) >= precision {
			break
		}
	}
	return result + fracStr
}
