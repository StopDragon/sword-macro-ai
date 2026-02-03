package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	gameDataEndpoint = "https://sword-ai.stopdragon.kr/api/game-data"
	cacheExpiry      = 1 * time.Hour
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

// 캐시된 게임 데이터
var (
	cachedData      *GameData
	cachedAt        time.Time
	cacheMu         sync.RWMutex
	dataInitialized bool
)

// FetchGameData 서버에서 게임 데이터 가져오기
func FetchGameData() (*GameData, error) {
	cacheMu.RLock()
	if cachedData != nil && time.Since(cachedAt) < cacheExpiry {
		defer cacheMu.RUnlock()
		return cachedData, nil
	}
	cacheMu.RUnlock()

	// 서버에서 데이터 가져오기
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(gameDataEndpoint)
	if err != nil {
		// 캐시가 있으면 만료되어도 사용
		cacheMu.RLock()
		if cachedData != nil {
			defer cacheMu.RUnlock()
			return cachedData, nil
		}
		cacheMu.RUnlock()
		return nil, fmt.Errorf("서버 연결 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cacheMu.RLock()
		if cachedData != nil {
			defer cacheMu.RUnlock()
			return cachedData, nil
		}
		cacheMu.RUnlock()
		return nil, fmt.Errorf("서버 오류: %d", resp.StatusCode)
	}

	var data GameData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("데이터 파싱 실패: %v", err)
	}

	// 캐시 업데이트
	cacheMu.Lock()
	cachedData = &data
	cachedAt = time.Now()
	dataInitialized = true
	cacheMu.Unlock()

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

// FormatGold 골드를 읽기 쉽게 포맷
func FormatGold(gold int) string {
	if gold >= 100000000 {
		return formatFloat(float64(gold)/100000000) + "억"
	}
	if gold >= 10000 {
		return formatFloat(float64(gold)/10000) + "만"
	}
	if gold >= 1000 {
		return formatFloat(float64(gold)/1000) + "천"
	}
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
