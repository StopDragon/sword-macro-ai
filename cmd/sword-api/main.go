package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

const (
	defaultAppSecret = "sw0rd-m4cr0-2026-s3cr3t-k3y" // 환경변수 없을 때 기본값
	appSecretEnvVar  = "SWORD_APP_SECRET"

	// 입력 검증 상수
	maxSessionIDLen  = 100
	maxAppVersionLen = 50
	maxOSTypeLen     = 20
	maxPeriodLen     = 20
	maxSwordNameLen  = 50
	maxMapEntries    = 1000    // 맵 최대 항목 수
	maxStatValue     = 1000000 // 단일 통계 최대값

	// Rate Limiting
	rateLimitWindow  = time.Minute
	rateLimitMax     = 60 // 분당 최대 요청
)

// getAppSecret 앱 시크릿 조회 (환경변수 우선)
func getAppSecret() string {
	if secret := os.Getenv(appSecretEnvVar); secret != "" {
		return secret
	}
	return defaultAppSecret
}

// Rate Limiter
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
}

var limiter = &rateLimiter{
	requests: make(map[string][]time.Time),
}

// isRateLimited IP 기반 Rate Limit 체크
func (rl *rateLimiter) isRateLimited(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rateLimitWindow)

	// 기존 요청 필터링 (윈도우 내의 것만 유지)
	var validRequests []time.Time
	for _, t := range rl.requests[ip] {
		if t.After(windowStart) {
			validRequests = append(validRequests, t)
		}
	}
	rl.requests[ip] = validRequests

	// Rate limit 체크
	if len(validRequests) >= rateLimitMax {
		return true
	}

	// 새 요청 기록
	rl.requests[ip] = append(rl.requests[ip], now)
	return false
}

// getClientIP 클라이언트 IP 추출
func getClientIP(r *http.Request) string {
	// X-Forwarded-For 헤더 우선
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// X-Real-IP 헤더
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

// ========================
// 게임 데이터 구조체
// ========================

type EnhanceRate struct {
	Level       int     `json:"level"`
	SuccessRate float64 `json:"success_rate"`
	KeepRate    float64 `json:"keep_rate"`
	DestroyRate float64 `json:"destroy_rate"`
}

type SwordPrice struct {
	Level    int `json:"level"`
	MinPrice int `json:"min_price"`
	MaxPrice int `json:"max_price"`
	AvgPrice int `json:"avg_price"`
}

type BattleReward struct {
	LevelDiff int     `json:"level_diff"`
	WinRate   float64 `json:"win_rate"`
	MinReward int     `json:"min_reward"`
	MaxReward int     `json:"max_reward"`
	AvgReward int     `json:"avg_reward"`
}

type GameData struct {
	EnhanceRates  []EnhanceRate  `json:"enhance_rates"`
	SwordPrices   []SwordPrice   `json:"sword_prices"`
	BattleRewards []BattleReward `json:"battle_rewards"`
	UpdatedAt     string         `json:"updated_at"`
}

// ========================
// 텔레메트리 구조체
// ========================

// === v1 통계 ===
type TelemetryStats struct {
	TotalCycles      int         `json:"total_cycles"`
	SuccessfulCycles int         `json:"successful_cycles"`
	FailedCycles     int         `json:"failed_cycles"`
	TotalGoldMined   int         `json:"total_gold_mined"`
	TotalSwordsFound int         `json:"total_swords_found"`
	SessionDuration  int         `json:"session_duration_sec"`
	EnhanceAttempts  int         `json:"enhance_attempts"`
	EnhanceSuccess   int         `json:"enhance_success"`
	EnhanceFail      int         `json:"enhance_fail"`
	EnhanceDestroy   int         `json:"enhance_destroy"`
	EnhanceByLevel   map[int]int `json:"enhance_by_level,omitempty"`
	BattleCount      int         `json:"battle_count"`
	BattleWins       int         `json:"battle_wins"`
	BattleLosses     int         `json:"battle_losses"`
	BattleGoldEarned int         `json:"battle_gold_earned"`
	UpsetWins        int         `json:"upset_wins"`
	UpsetAttempts    int         `json:"upset_attempts"`
	SalesCount       int         `json:"sales_count"`
	SalesTotalGold   int         `json:"sales_total_gold"`
	SalesMaxPrice    int         `json:"sales_max_price"`
	FarmingAttempts  int         `json:"farming_attempts"`
	SpecialFound     int         `json:"special_found"`
	TrashFound       int         `json:"trash_found"`

	// === v2 새로 추가 ===
	SwordBattleStats   map[string]*SwordBattleStat  `json:"sword_battle_stats,omitempty"`
	SpecialFoundByName map[string]int               `json:"special_found_by_name,omitempty"`
	UpsetStatsByDiff  map[int]*UpsetStat           `json:"upset_stats_by_diff,omitempty"`
	SwordSaleStats    map[string]*SwordSaleStat    `json:"sword_sale_stats,omitempty"`
	SwordEnhanceStats map[string]*SwordEnhanceStat `json:"sword_enhance_stats,omitempty"`
	ItemFarmingStats  map[string]*ItemFarmingStat  `json:"item_farming_stats,omitempty"`

	// === v3 새로 추가 ===
	EnhanceLevelDetail map[int]*EnhanceLevelStat `json:"enhance_level_detail,omitempty"`
	EnhanceCostTotal   int                        `json:"enhance_cost_total"`
	CycleTimeTotal     float64                    `json:"cycle_time_total"`
	BattleGoldLost     int                        `json:"battle_gold_lost"`
}

// === v2 구조체들 ===

// SwordBattleStat 검 종류별 배틀 통계
type SwordBattleStat struct {
	BattleCount   int `json:"battle_count"`
	BattleWins    int `json:"battle_wins"`
	UpsetAttempts int `json:"upset_attempts"`
	UpsetWins     int `json:"upset_wins"`
}

// UpsetStat 레벨차별 역배 통계
type UpsetStat struct {
	Attempts   int `json:"attempts"`
	Wins       int `json:"wins"`
	GoldEarned int `json:"gold_earned"`
}

// SwordSaleStat 검 종류별 판매 통계
type SwordSaleStat struct {
	TotalPrice int `json:"total_price"`
	Count      int `json:"count"`
}

// SwordEnhanceStat 검 종류별 강화 통계
type SwordEnhanceStat struct {
	Attempts int `json:"attempts"`
	Success  int `json:"success"`
	Fail     int `json:"fail"`
	Destroy  int `json:"destroy"`
}

// ItemFarmingStat 아이템별 파밍 통계
type ItemFarmingStat struct {
	TotalCount   int `json:"total_count"`
	SpecialCount int `json:"special_count"`
	NormalCount  int `json:"normal_count"`
	TrashCount   int `json:"trash_count"`
}

// === v3 구조체들 ===

// EnhanceLevelStat 레벨별 강화 상세 통계
type EnhanceLevelStat struct {
	Attempts int `json:"attempts"`
	Success  int `json:"success"`
	Fail     int `json:"fail"`
	Destroy  int `json:"destroy"`
}

type TelemetryPayload struct {
	SchemaVersion int            `json:"schema_version"`
	AppVersion    string         `json:"app_version"`
	OSType        string         `json:"os_type"`
	SessionID     string         `json:"session_id"`
	Period        string         `json:"period"`
	Mode          string         `json:"mode,omitempty"` // v3: 현재 모드
	Stats         TelemetryStats `json:"stats"`
}

// ========================
// 통계 저장소
// ========================

type StatsStore struct {
	mu              sync.RWMutex
	enhanceAttempts int
	enhanceSuccess  int
	enhanceFail     int
	enhanceDestroy  int
	enhanceByLevel  map[int]int
	battleCount     int
	battleWins      int
	upsetAttempts   int
	upsetWins       int
	battleGold      int
	farmingAttempts int
	specialFound    int
	salesCount      int
	salesTotalGold  int

	// === v2 통계 ===
	swordBattleStats   map[string]*SwordBattleStat
	specialFoundByName map[string]int
	upsetStatsByDiff  map[int]*UpsetStat
	swordSaleStats    map[string]*SwordSaleStat
	swordEnhanceStats map[string]*SwordEnhanceStat
	itemFarmingStats  map[string]*ItemFarmingStat

	// === v3 통계 ===
	enhanceLevelDetail map[int]*EnhanceLevelStat
	enhanceCostTotal   int
	cycleTimeTotal     float64
	battleGoldLost     int
}

var stats = &StatsStore{
	enhanceByLevel:     make(map[int]int),
	swordBattleStats:   make(map[string]*SwordBattleStat),
	specialFoundByName: make(map[string]int),
	upsetStatsByDiff:   make(map[int]*UpsetStat),
	swordSaleStats:     make(map[string]*SwordSaleStat),
	swordEnhanceStats:  make(map[string]*SwordEnhanceStat),
	itemFarmingStats:   make(map[string]*ItemFarmingStat),
	enhanceLevelDetail: make(map[int]*EnhanceLevelStat),
}

// ========================
// 게임 데이터 (실측 통계 + 기본값 혼합)
// ========================

const minSampleSize = 10 // 실측 데이터 사용 최소 샘플 수

// 기본 강화 확률 (실측 데이터 부족 시 사용)
var defaultEnhanceRates = []EnhanceRate{
	{Level: 0, SuccessRate: 100.0, KeepRate: 0.0, DestroyRate: 0.0},
	{Level: 1, SuccessRate: 95.0, KeepRate: 5.0, DestroyRate: 0.0},
	{Level: 2, SuccessRate: 90.0, KeepRate: 10.0, DestroyRate: 0.0},
	{Level: 3, SuccessRate: 85.0, KeepRate: 15.0, DestroyRate: 0.0},
	{Level: 4, SuccessRate: 80.0, KeepRate: 20.0, DestroyRate: 0.0},
	{Level: 5, SuccessRate: 70.0, KeepRate: 25.0, DestroyRate: 5.0},
	{Level: 6, SuccessRate: 60.0, KeepRate: 30.0, DestroyRate: 10.0},
	{Level: 7, SuccessRate: 50.0, KeepRate: 35.0, DestroyRate: 15.0},
	{Level: 8, SuccessRate: 40.0, KeepRate: 40.0, DestroyRate: 20.0},
	{Level: 9, SuccessRate: 30.0, KeepRate: 45.0, DestroyRate: 25.0},
	{Level: 10, SuccessRate: 25.0, KeepRate: 45.0, DestroyRate: 30.0},
	{Level: 11, SuccessRate: 20.0, KeepRate: 45.0, DestroyRate: 35.0},
	{Level: 12, SuccessRate: 15.0, KeepRate: 45.0, DestroyRate: 40.0},
	{Level: 13, SuccessRate: 10.0, KeepRate: 45.0, DestroyRate: 45.0},
	{Level: 14, SuccessRate: 5.0, KeepRate: 45.0, DestroyRate: 50.0},
}

// 기본 배틀 보상 (실측 데이터 부족 시 사용)
var defaultBattleRewards = []BattleReward{
	{LevelDiff: 1, WinRate: 35.0, MinReward: 500, MaxReward: 1500, AvgReward: 1000},
	{LevelDiff: 2, WinRate: 20.0, MinReward: 1500, MaxReward: 4000, AvgReward: 2750},
	{LevelDiff: 3, WinRate: 10.0, MinReward: 4000, MaxReward: 10000, AvgReward: 7000},
	{LevelDiff: 4, WinRate: 5.0, MinReward: 10000, MaxReward: 25000, AvgReward: 17500},
	{LevelDiff: 5, WinRate: 3.0, MinReward: 25000, MaxReward: 60000, AvgReward: 42500},
	{LevelDiff: 6, WinRate: 2.0, MinReward: 60000, MaxReward: 140000, AvgReward: 100000},
	{LevelDiff: 7, WinRate: 1.5, MinReward: 140000, MaxReward: 300000, AvgReward: 220000},
	{LevelDiff: 8, WinRate: 1.0, MinReward: 300000, MaxReward: 600000, AvgReward: 450000},
	{LevelDiff: 9, WinRate: 0.7, MinReward: 600000, MaxReward: 1200000, AvgReward: 900000},
	{LevelDiff: 10, WinRate: 0.5, MinReward: 1200000, MaxReward: 2500000, AvgReward: 1850000},
	{LevelDiff: 11, WinRate: 0.35, MinReward: 2500000, MaxReward: 5000000, AvgReward: 3750000},
	{LevelDiff: 12, WinRate: 0.25, MinReward: 5000000, MaxReward: 10000000, AvgReward: 7500000},
	{LevelDiff: 13, WinRate: 0.18, MinReward: 10000000, MaxReward: 20000000, AvgReward: 15000000},
	{LevelDiff: 14, WinRate: 0.12, MinReward: 20000000, MaxReward: 40000000, AvgReward: 30000000},
	{LevelDiff: 15, WinRate: 0.08, MinReward: 40000000, MaxReward: 80000000, AvgReward: 60000000},
	{LevelDiff: 16, WinRate: 0.05, MinReward: 80000000, MaxReward: 150000000, AvgReward: 115000000},
	{LevelDiff: 17, WinRate: 0.03, MinReward: 150000000, MaxReward: 300000000, AvgReward: 225000000},
	{LevelDiff: 18, WinRate: 0.02, MinReward: 300000000, MaxReward: 500000000, AvgReward: 400000000},
	{LevelDiff: 19, WinRate: 0.01, MinReward: 500000000, MaxReward: 800000000, AvgReward: 650000000},
	{LevelDiff: 20, WinRate: 0.005, MinReward: 800000000, MaxReward: 1000000000, AvgReward: 900000000},
}

// 기본 판매가 (게임에서 정해진 값)
var defaultSwordPrices = []SwordPrice{
	{Level: 0, MinPrice: 10, MaxPrice: 20, AvgPrice: 15},
	{Level: 1, MinPrice: 30, MaxPrice: 50, AvgPrice: 40},
	{Level: 2, MinPrice: 80, MaxPrice: 120, AvgPrice: 100},
	{Level: 3, MinPrice: 200, MaxPrice: 300, AvgPrice: 250},
	{Level: 4, MinPrice: 500, MaxPrice: 700, AvgPrice: 600},
	{Level: 5, MinPrice: 1000, MaxPrice: 1500, AvgPrice: 1250},
	{Level: 6, MinPrice: 2500, MaxPrice: 3500, AvgPrice: 3000},
	{Level: 7, MinPrice: 6000, MaxPrice: 8000, AvgPrice: 7000},
	{Level: 8, MinPrice: 15000, MaxPrice: 20000, AvgPrice: 17500},
	{Level: 9, MinPrice: 40000, MaxPrice: 55000, AvgPrice: 47500},
	{Level: 10, MinPrice: 100000, MaxPrice: 140000, AvgPrice: 120000},
	{Level: 11, MinPrice: 280000, MaxPrice: 350000, AvgPrice: 315000},
	{Level: 12, MinPrice: 800000, MaxPrice: 1000000, AvgPrice: 900000},
	{Level: 13, MinPrice: 2500000, MaxPrice: 3200000, AvgPrice: 2850000},
	{Level: 14, MinPrice: 8000000, MaxPrice: 10000000, AvgPrice: 9000000},
	{Level: 15, MinPrice: 30000000, MaxPrice: 40000000, AvgPrice: 35000000},
}

// extractTypeLevel 키에서 타입과 레벨 추출
// 키 형식: "{type}_{level}" (예: "normal_10", "special_5", "trash_3")
// 반환: (타입, 레벨, 성공여부)
func extractTypeLevel(key string) (string, int, bool) {
	parts := strings.Split(key, "_")
	if len(parts) < 2 {
		return "", 0, false
	}

	// 마지막 부분이 레벨 숫자
	levelStr := parts[len(parts)-1]
	level, err := strconv.Atoi(levelStr)
	if err != nil {
		return "", 0, false
	}

	// 나머지가 타입 (normal, special, trash만 허용)
	itemType := strings.Join(parts[:len(parts)-1], "_")
	if itemType != "normal" && itemType != "special" && itemType != "trash" {
		return "", 0, false
	}

	return itemType, level, true
}

func getGameData() GameData {
	stats.mu.RLock()
	defer stats.mu.RUnlock()

	// 강화 확률: 실측 데이터 반영
	enhanceRates := make([]EnhanceRate, len(defaultEnhanceRates))
	copy(enhanceRates, defaultEnhanceRates)

	// v3: 레벨별 강화 상세 통계가 있으면 실측 확률로 대체
	for i := range enhanceRates {
		lvl := enhanceRates[i].Level
		if detail, ok := stats.enhanceLevelDetail[lvl]; ok && detail.Attempts >= minSampleSize {
			total := float64(detail.Attempts)
			enhanceRates[i].SuccessRate = float64(detail.Success) / total * 100
			enhanceRates[i].KeepRate = float64(detail.Fail) / total * 100
			enhanceRates[i].DestroyRate = float64(detail.Destroy) / total * 100
		}
	}

	// 배틀 보상: 실측 승률 반영
	battleRewards := make([]BattleReward, len(defaultBattleRewards))
	copy(battleRewards, defaultBattleRewards)

	for i := range battleRewards {
		diff := battleRewards[i].LevelDiff
		if upsetStat, ok := stats.upsetStatsByDiff[diff]; ok && upsetStat.Attempts >= minSampleSize {
			// 실측 승률로 대체
			realWinRate := float64(upsetStat.Wins) / float64(upsetStat.Attempts) * 100
			battleRewards[i].WinRate = realWinRate

			// 실측 평균 보상으로 대체 (승리 시에만 보상이 있으므로)
			if upsetStat.Wins > 0 {
				battleRewards[i].AvgReward = upsetStat.GoldEarned / upsetStat.Wins
			}
		}
	}

	// 검 가격: 실측 판매 데이터 반영
	swordPrices := make([]SwordPrice, len(defaultSwordPrices))
	copy(swordPrices, defaultSwordPrices)

	// swordSaleStats에서 레벨별 판매 통계 집계
	// 키 형식: "{검이름}_{레벨}" (예: "불꽃검_10", "검_8")
	levelSales := make(map[int]struct {
		totalPrice int
		count      int
	})
	for key, stat := range stats.swordSaleStats {
		// 키에서 레벨 추출 (마지막 "_" 뒤의 숫자)
		parts := strings.Split(key, "_")
		if len(parts) < 2 {
			continue
		}
		levelStr := parts[len(parts)-1]
		level, err := strconv.Atoi(levelStr)
		if err != nil {
			continue
		}
		// 레벨별로 집계
		entry := levelSales[level]
		entry.totalPrice += stat.TotalPrice
		entry.count += stat.Count
		levelSales[level] = entry
	}

	// 실측 평균 가격으로 대체 (minSampleSize 이상일 때만)
	for i := range swordPrices {
		lvl := swordPrices[i].Level
		if entry, ok := levelSales[lvl]; ok && entry.count >= minSampleSize {
			realAvgPrice := entry.totalPrice / entry.count
			swordPrices[i].AvgPrice = realAvgPrice
			// MinPrice, MaxPrice도 실측 기준으로 추정 (±20%)
			swordPrices[i].MinPrice = int(float64(realAvgPrice) * 0.8)
			swordPrices[i].MaxPrice = int(float64(realAvgPrice) * 1.2)
		}
	}

	return GameData{
		EnhanceRates:  enhanceRates,
		SwordPrices:   swordPrices,
		BattleRewards: battleRewards,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
}

// ========================
// API 핸들러
// ========================

func handleGameData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	data := getGameData()
	json.NewEncoder(w).Encode(data)
}

func handleTelemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-App-Signature")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Rate Limiting
	clientIP := getClientIP(r)
	if limiter.isRateLimited(clientIP) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// 서명 검증
	signature := r.Header.Get("X-App-Signature")
	if signature == "" {
		http.Error(w, "Missing signature", http.StatusUnauthorized)
		return
	}

	var payload TelemetryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// 입력 검증
	if err := validateTelemetryPayload(&payload); err != nil {
		log.Printf("[텔레메트리] 검증 실패: %v (IP=%s)", err, clientIP)
		http.Error(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 서명 검증
	expectedSig := generateSignature(payload.SessionID, payload.Period)
	if signature != expectedSig {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// 통계 업데이트
	stats.mu.Lock()
	// v1 통계
	stats.enhanceAttempts += payload.Stats.EnhanceAttempts
	stats.enhanceSuccess += payload.Stats.EnhanceSuccess
	stats.enhanceFail += payload.Stats.EnhanceFail
	stats.enhanceDestroy += payload.Stats.EnhanceDestroy
	for lvl, cnt := range payload.Stats.EnhanceByLevel {
		stats.enhanceByLevel[lvl] += cnt
	}
	stats.battleCount += payload.Stats.BattleCount
	stats.battleWins += payload.Stats.BattleWins
	stats.upsetAttempts += payload.Stats.UpsetAttempts
	stats.upsetWins += payload.Stats.UpsetWins
	stats.battleGold += payload.Stats.BattleGoldEarned
	stats.farmingAttempts += payload.Stats.FarmingAttempts
	stats.specialFound += payload.Stats.SpecialFound
	stats.salesCount += payload.Stats.SalesCount
	stats.salesTotalGold += payload.Stats.SalesTotalGold

	// v2 통계 (schema_version >= 2)
	if payload.SchemaVersion >= 2 {
		// 검 종류별 배틀 통계
		for name, stat := range payload.Stats.SwordBattleStats {
			if stats.swordBattleStats[name] == nil {
				stats.swordBattleStats[name] = &SwordBattleStat{}
			}
			stats.swordBattleStats[name].BattleCount += stat.BattleCount
			stats.swordBattleStats[name].BattleWins += stat.BattleWins
			stats.swordBattleStats[name].UpsetAttempts += stat.UpsetAttempts
			stats.swordBattleStats[name].UpsetWins += stat.UpsetWins
		}

		// 특수 이름별 통계
		for name, cnt := range payload.Stats.SpecialFoundByName {
			stats.specialFoundByName[name] += cnt
		}

		// 레벨차별 역배 통계
		for diff, stat := range payload.Stats.UpsetStatsByDiff {
			if stats.upsetStatsByDiff[diff] == nil {
				stats.upsetStatsByDiff[diff] = &UpsetStat{}
			}
			stats.upsetStatsByDiff[diff].Attempts += stat.Attempts
			stats.upsetStatsByDiff[diff].Wins += stat.Wins
			stats.upsetStatsByDiff[diff].GoldEarned += stat.GoldEarned
		}

		// 검 판매 통계
		for key, stat := range payload.Stats.SwordSaleStats {
			if stats.swordSaleStats[key] == nil {
				stats.swordSaleStats[key] = &SwordSaleStat{}
			}
			stats.swordSaleStats[key].TotalPrice += stat.TotalPrice
			stats.swordSaleStats[key].Count += stat.Count
		}

		// 검 강화 통계
		for name, stat := range payload.Stats.SwordEnhanceStats {
			if stats.swordEnhanceStats[name] == nil {
				stats.swordEnhanceStats[name] = &SwordEnhanceStat{}
			}
			stats.swordEnhanceStats[name].Attempts += stat.Attempts
			stats.swordEnhanceStats[name].Success += stat.Success
			stats.swordEnhanceStats[name].Fail += stat.Fail
			stats.swordEnhanceStats[name].Destroy += stat.Destroy
		}

		// 아이템 파밍 통계
		for name, stat := range payload.Stats.ItemFarmingStats {
			if stats.itemFarmingStats[name] == nil {
				stats.itemFarmingStats[name] = &ItemFarmingStat{}
			}
			stats.itemFarmingStats[name].TotalCount += stat.TotalCount
			stats.itemFarmingStats[name].SpecialCount += stat.SpecialCount
			stats.itemFarmingStats[name].NormalCount += stat.NormalCount
			stats.itemFarmingStats[name].TrashCount += stat.TrashCount
		}
	}

	// v3 통계 (schema_version >= 3)
	if payload.SchemaVersion >= 3 {
		// 레벨별 강화 상세 통계
		for lvl, stat := range payload.Stats.EnhanceLevelDetail {
			if stats.enhanceLevelDetail[lvl] == nil {
				stats.enhanceLevelDetail[lvl] = &EnhanceLevelStat{}
			}
			stats.enhanceLevelDetail[lvl].Attempts += stat.Attempts
			stats.enhanceLevelDetail[lvl].Success += stat.Success
			stats.enhanceLevelDetail[lvl].Fail += stat.Fail
			stats.enhanceLevelDetail[lvl].Destroy += stat.Destroy
		}

		stats.enhanceCostTotal += payload.Stats.EnhanceCostTotal
		stats.cycleTimeTotal += payload.Stats.CycleTimeTotal
		stats.battleGoldLost += payload.Stats.BattleGoldLost
	}
	stats.mu.Unlock()

	// SQLite에 영구 저장
	if db != nil {
		go saveToDB()
	}

	modeStr := payload.Mode
	if modeStr == "" {
		modeStr = "-"
	}
	log.Printf("[텔레메트리] 세션=%s 버전=%s OS=%s 모드=%s v%d", payload.SessionID[:8], payload.AppVersion, payload.OSType, modeStr, payload.SchemaVersion)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleStatsDetailed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	// 강화 통계
	enhanceTotal := stats.enhanceSuccess + stats.enhanceFail + stats.enhanceDestroy
	successRate := "0%"
	keepRate := "0%"
	destroyRate := "0%"
	if enhanceTotal > 0 {
		successRate = fmt.Sprintf("%.1f%%", float64(stats.enhanceSuccess)/float64(enhanceTotal)*100)
		keepRate = fmt.Sprintf("%.1f%%", float64(stats.enhanceFail)/float64(enhanceTotal)*100)
		destroyRate = fmt.Sprintf("%.1f%%", float64(stats.enhanceDestroy)/float64(enhanceTotal)*100)
	}

	// 배틀 통계
	battleWinRate := "0%"
	upsetWinRate := "0%"
	avgBattleGold := 0
	if stats.battleCount > 0 {
		battleWinRate = fmt.Sprintf("%.1f%%", float64(stats.battleWins)/float64(stats.battleCount)*100)
		avgBattleGold = stats.battleGold / stats.battleCount
	}
	if stats.upsetAttempts > 0 {
		upsetWinRate = fmt.Sprintf("%.1f%%", float64(stats.upsetWins)/float64(stats.upsetAttempts)*100)
	}

	// 파밍 통계
	specialRate := "0%"
	if stats.farmingAttempts > 0 {
		specialRate = fmt.Sprintf("%.2f%%", float64(stats.specialFound)/float64(stats.farmingAttempts)*100)
	}

	// 판매 통계
	avgSalePrice := 0
	if stats.salesCount > 0 {
		avgSalePrice = stats.salesTotalGold / stats.salesCount
	}

	result := map[string]interface{}{
		"강화": map[string]interface{}{
			"총_시도":    enhanceTotal,
			"성공률":     successRate,
			"유지율":     keepRate,
			"파괴율":     destroyRate,
			"레벨별_성공": stats.enhanceByLevel,
		},
		"배틀": map[string]interface{}{
			"총_대결":   stats.battleCount,
			"승률":     battleWinRate,
			"역배_시도": stats.upsetAttempts,
			"역배_승률": upsetWinRate,
			"총_전리품": fmt.Sprintf("%dG", stats.battleGold),
			"평균_전리품": fmt.Sprintf("%dG", avgBattleGold),
		},
		"파밍": map[string]interface{}{
			"총_시도":  stats.farmingAttempts,
			"특수_확률": specialRate,
		},
		"판매": map[string]interface{}{
			"총_판매": stats.salesCount,
			"총_수익": fmt.Sprintf("%dG", stats.salesTotalGold),
			"평균_가격": fmt.Sprintf("%dG", avgSalePrice),
		},
	}

	json.NewEncoder(w).Encode(result)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// === v2 API 엔드포인트 ===

// 검 종류별 승률 랭킹
func handleSwordStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type SwordEntry struct {
		Name         string  `json:"name"`
		BattleCount  int     `json:"battle_count"`
		WinRate      float64 `json:"win_rate"`
		UpsetWinRate float64 `json:"upset_win_rate"`
	}

	var swords []SwordEntry
	for name, stat := range stats.swordBattleStats {
		winRate := 0.0
		upsetWinRate := 0.0
		if stat.BattleCount > 0 {
			winRate = float64(stat.BattleWins) / float64(stat.BattleCount) * 100
		}
		if stat.UpsetAttempts > 0 {
			upsetWinRate = float64(stat.UpsetWins) / float64(stat.UpsetAttempts) * 100
		}
		swords = append(swords, SwordEntry{
			Name:         name,
			BattleCount:  stat.BattleCount,
			WinRate:      winRate,
			UpsetWinRate: upsetWinRate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"swords": swords,
	})
}

// 특수 검 출현 확률
func handleSpecialStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type SpecialEntry struct {
		Name  string  `json:"name"`
		Count int     `json:"count"`
		Rate  float64 `json:"rate"`
	}

	var specials []SpecialEntry
	for name, cnt := range stats.specialFoundByName {
		rate := 0.0
		if stats.farmingAttempts > 0 {
			rate = float64(cnt) / float64(stats.farmingAttempts) * 100
		}
		specials = append(specials, SpecialEntry{
			Name:  name,
			Count: cnt,
			Rate:  rate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_farming": stats.farmingAttempts,
		"special":       specials,
	})
}

// 역배 실측 승률
func handleUpsetStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	// 이론 승률: defaultBattleRewards에서 추출
	theoryRates := make(map[int]float64)
	for _, br := range defaultBattleRewards {
		theoryRates[br.LevelDiff] = br.WinRate
	}

	type DiffStat struct {
		Attempts   int     `json:"attempts"`
		Wins       int     `json:"wins"`
		WinRate    float64 `json:"win_rate"`
		Theory     float64 `json:"theory"`
		GoldEarned int     `json:"gold_earned"`
	}

	byDiff := make(map[string]DiffStat)
	for diff := 1; diff <= 20; diff++ {
		stat := stats.upsetStatsByDiff[diff]
		winRate := 0.0
		attempts := 0
		wins := 0
		gold := 0
		if stat != nil {
			attempts = stat.Attempts
			wins = stat.Wins
			gold = stat.GoldEarned
			if attempts > 0 {
				winRate = float64(wins) / float64(attempts) * 100
			}
		}
		byDiff[fmt.Sprintf("%d", diff)] = DiffStat{
			Attempts:   attempts,
			Wins:       wins,
			WinRate:    winRate,
			Theory:     theoryRates[diff],
			GoldEarned: gold,
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"by_level_diff": byDiff,
	})
}

// 아이템 파밍 통계
func handleItemStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type ItemEntry struct {
		Name         string  `json:"name"`
		TotalCount   int     `json:"total_count"`
		SpecialCount int     `json:"special_count"`
		NormalCount  int     `json:"normal_count"`
		SpecialRate  float64 `json:"special_rate"`
	}

	var items []ItemEntry
	for name, stat := range stats.itemFarmingStats {
		specialRate := 0.0
		if stat.TotalCount > 0 {
			specialRate = float64(stat.SpecialCount) / float64(stat.TotalCount) * 100
		}
		items = append(items, ItemEntry{
			Name:         name,
			TotalCount:   stat.TotalCount,
			SpecialCount: stat.SpecialCount,
			NormalCount:  stat.NormalCount,
			SpecialRate:  specialRate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_farming": stats.farmingAttempts,
		"items":         items,
	})
}

// 검 종류별 강화 성공률
func handleEnhanceStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type EnhanceEntry struct {
		Name        string  `json:"name"`
		Attempts    int     `json:"attempts"`
		Success     int     `json:"success"`
		Fail        int     `json:"fail"`
		Destroy     int     `json:"destroy"`
		SuccessRate float64 `json:"success_rate"`
		DestroyRate float64 `json:"destroy_rate"`
	}

	var swords []EnhanceEntry
	for name, stat := range stats.swordEnhanceStats {
		successRate := 0.0
		destroyRate := 0.0
		if stat.Attempts > 0 {
			successRate = float64(stat.Success) / float64(stat.Attempts) * 100
			destroyRate = float64(stat.Destroy) / float64(stat.Attempts) * 100
		}
		swords = append(swords, EnhanceEntry{
			Name:        name,
			Attempts:    stat.Attempts,
			Success:     stat.Success,
			Fail:        stat.Fail,
			Destroy:     stat.Destroy,
			SuccessRate: successRate,
			DestroyRate: destroyRate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_attempts": stats.enhanceAttempts,
		"total_success":  stats.enhanceSuccess,
		"total_destroy":  stats.enhanceDestroy,
		"swords":         swords,
	})
}

// 검 종류+레벨별 판매 통계
func handleSaleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type SaleEntry struct {
		Key        string `json:"key"`        // "검이름_레벨"
		TotalPrice int    `json:"total_price"`
		Count      int    `json:"count"`
		AvgPrice   int    `json:"avg_price"`
	}

	var sales []SaleEntry
	totalCount := 0
	totalGold := 0

	for key, stat := range stats.swordSaleStats {
		avgPrice := 0
		if stat.Count > 0 {
			avgPrice = stat.TotalPrice / stat.Count
		}
		sales = append(sales, SaleEntry{
			Key:        key,
			TotalPrice: stat.TotalPrice,
			Count:      stat.Count,
			AvgPrice:   avgPrice,
		})
		totalCount += stat.Count
		totalGold += stat.TotalPrice
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_sales": totalCount,
		"total_gold":  totalGold,
		"sales":       sales,
	})
}

// 최적 판매 시점 계산 (시간 효율 기반)
func handleOptimalSellPoint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	gameData := getGameData()

	// 레벨별 예상 강화 횟수 계산 (0부터 해당 레벨까지)
	// 기대 시도 횟수 = Σ(1 / 성공률)
	calcExpectedTrials := func(targetLevel int) float64 {
		if targetLevel <= 0 {
			return 0
		}
		total := 0.0
		for lvl := 0; lvl < targetLevel && lvl < len(gameData.EnhanceRates); lvl++ {
			rate := gameData.EnhanceRates[lvl].SuccessRate / 100.0
			if rate > 0 {
				total += 1.0 / rate
			}
		}
		return total
	}

	// 예상 시간 계산 (초 단위)
	// 실제 클라이언트 설정 기반:
	// - TrashDelay: 1.2초 (파밍/판매 후)
	// - LowDelay: 1.5초 (0-8강)
	// - MidDelay: 2.5초 (9강)
	// - HighDelay: 3.5초 (10강+)
	// + 응답 대기/처리 오버헤드: 약 1초
	calcExpectedTime := func(targetLevel int) float64 {
		const (
			farmTime     = 1.2 // TrashDelay (판매 후 새 검 받기)
			lowDelay     = 2.5 // LowDelay(1.5) + 응답대기(1.0)
			midDelay     = 3.5 // MidDelay(2.5) + 응답대기(1.0)
			highDelay    = 4.5 // HighDelay(3.5) + 응답대기(1.0)
			slowdownLvl  = 9   // SlowdownLevel
		)

		totalTime := farmTime
		for lvl := 0; lvl < targetLevel && lvl < len(gameData.EnhanceRates); lvl++ {
			rate := gameData.EnhanceRates[lvl].SuccessRate / 100.0
			if rate <= 0 {
				continue
			}
			expectedTries := 1.0 / rate

			// 레벨별 딜레이 적용
			var delay float64
			if lvl >= 10 {
				delay = highDelay
			} else if lvl >= slowdownLvl {
				delay = midDelay
			} else {
				delay = lowDelay
			}
			totalTime += expectedTries * delay
		}
		return totalTime
	}

	type LevelEfficiency struct {
		Level              int     `json:"level"`
		AvgPrice           int     `json:"avg_price"`
		ExpectedTrials     float64 `json:"expected_trials"`     // 기대 강화 횟수
		ExpectedTimeSecond float64 `json:"expected_time_second"` // 기대 소요 시간
		SuccessProb        float64 `json:"success_prob"`        // 성공 확률 (%)
		GoldPerMinute      float64 `json:"gold_per_minute"`     // 시간당 골드 효율
		Recommendation     string  `json:"recommendation"`       // 추천 여부
	}

	var efficiencies []LevelEfficiency
	bestLevel := 10
	bestGPM := 0.0

	// 레벨 5-15 범위에서 분석
	for level := 5; level <= 15 && level < len(gameData.SwordPrices); level++ {
		price := gameData.SwordPrices[level].AvgPrice
		trials := calcExpectedTrials(level)
		timeSeconds := calcExpectedTime(level)

		// 성공 확률 (0부터 해당 레벨까지)
		successProb := 1.0
		for lvl := 0; lvl < level && lvl < len(gameData.EnhanceRates); lvl++ {
			successProb *= gameData.EnhanceRates[lvl].SuccessRate / 100.0
		}

		// 시간당 골드 효율 = (판매가 × 성공확률) / (소요시간/60)
		gpm := 0.0
		if timeSeconds > 0 {
			gpm = (float64(price) * successProb) / (timeSeconds / 60.0)
		}

		recommendation := ""
		if gpm > bestGPM {
			bestGPM = gpm
			bestLevel = level
		}

		efficiencies = append(efficiencies, LevelEfficiency{
			Level:              level,
			AvgPrice:           price,
			ExpectedTrials:     trials,
			ExpectedTimeSecond: timeSeconds,
			SuccessProb:        successProb * 100,
			GoldPerMinute:      gpm,
			Recommendation:     recommendation,
		})
	}

	// 최적 레벨에 추천 표시
	for i := range efficiencies {
		if efficiencies[i].Level == bestLevel {
			efficiencies[i].Recommendation = "optimal"
		}
	}

	// 타입별 판매가 집계 (normal_10, special_10 등에서 추출)
	typeLevelPrices := make(map[string]map[int]struct {
		totalPrice int
		count      int
	})
	for key, stat := range stats.swordSaleStats {
		itemType, level, ok := extractTypeLevel(key)
		if !ok {
			continue
		}
		if typeLevelPrices[itemType] == nil {
			typeLevelPrices[itemType] = make(map[int]struct {
				totalPrice int
				count      int
			})
		}
		entry := typeLevelPrices[itemType][level]
		entry.totalPrice += stat.TotalPrice
		entry.count += stat.Count
		typeLevelPrices[itemType][level] = entry
	}

	// 타입별 강화 확률 집계 (normal_10, special_10 등에서 추출)
	typeLevelEnhance := make(map[string]map[int]struct {
		attempts int
		success  int
	})
	for key, stat := range stats.swordEnhanceStats {
		itemType, level, ok := extractTypeLevel(key)
		if !ok {
			continue
		}
		if typeLevelEnhance[itemType] == nil {
			typeLevelEnhance[itemType] = make(map[int]struct {
				attempts int
				success  int
			})
		}
		entry := typeLevelEnhance[itemType][level]
		entry.attempts += stat.Attempts
		entry.success += stat.Success
		typeLevelEnhance[itemType][level] = entry
	}

	// 타입별 강화 성공률 계산 (샘플 부족 시 기본값 사용)
	getEnhanceRateForType := func(itemType string, level int) float64 {
		if typeData, ok := typeLevelEnhance[itemType]; ok {
			if entry, ok := typeData[level]; ok && entry.attempts >= minSampleSize {
				return float64(entry.success) / float64(entry.attempts)
			}
		}
		// 기본값 사용
		if level < len(gameData.EnhanceRates) {
			return gameData.EnhanceRates[level].SuccessRate / 100.0
		}
		return 0.05 // 매우 낮은 기본값
	}

	// 타입별 평균 판매가 계산 (샘플 부족 시 기본값 사용)
	getAvgPriceForType := func(itemType string, level int) int {
		if typeData, ok := typeLevelPrices[itemType]; ok {
			if entry, ok := typeData[level]; ok && entry.count >= minSampleSize {
				return entry.totalPrice / entry.count
			}
		}
		// 기본값 사용
		if level < len(gameData.SwordPrices) {
			return gameData.SwordPrices[level].AvgPrice
		}
		return 0
	}

	// 타입별 예상 시간 계산
	calcExpectedTimeForType := func(itemType string, targetLevel int) float64 {
		const (
			farmTime    = 1.2
			lowDelay    = 2.5
			midDelay    = 3.5
			highDelay   = 4.5
			slowdownLvl = 9
		)

		totalTime := farmTime
		for lvl := 0; lvl < targetLevel; lvl++ {
			rate := getEnhanceRateForType(itemType, lvl)
			if rate <= 0 {
				continue
			}
			expectedTries := 1.0 / rate

			var delay float64
			if lvl >= 10 {
				delay = highDelay
			} else if lvl >= slowdownLvl {
				delay = midDelay
			} else {
				delay = lowDelay
			}
			totalTime += expectedTries * delay
		}
		return totalTime
	}

	// 타입별 최적 레벨 계산
	type TypeOptimal struct {
		Type           string  `json:"type"`
		OptimalLevel   int     `json:"optimal_level"`
		OptimalGPM     float64 `json:"optimal_gpm"`
		SampleSize     int     `json:"sample_size"`
		EnhanceSamples int     `json:"enhance_samples"`
		IsDefault      bool    `json:"is_default"`
	}

	calcTypeOptimal := func(itemType string) TypeOptimal {
		bestLvl := 10
		bestGpm := 0.0
		totalSales := 0
		totalEnhance := 0

		// 해당 타입의 총 샘플 수 계산
		if typeData, ok := typeLevelPrices[itemType]; ok {
			for _, entry := range typeData {
				totalSales += entry.count
			}
		}
		if typeData, ok := typeLevelEnhance[itemType]; ok {
			for _, entry := range typeData {
				totalEnhance += entry.attempts
			}
		}

		isDefault := totalSales < minSampleSize || totalEnhance < minSampleSize

		for level := 5; level <= 15; level++ {
			price := getAvgPriceForType(itemType, level)
			timeSeconds := calcExpectedTimeForType(itemType, level)

			// 성공 확률 계산
			successProb := 1.0
			for lvl := 0; lvl < level; lvl++ {
				successProb *= getEnhanceRateForType(itemType, lvl)
			}

			gpm := 0.0
			if timeSeconds > 0 {
				gpm = (float64(price) * successProb) / (timeSeconds / 60.0)
			}

			if gpm > bestGpm {
				bestGpm = gpm
				bestLvl = level
			}
		}

		return TypeOptimal{
			Type:           itemType,
			OptimalLevel:   bestLvl,
			OptimalGPM:     bestGpm,
			SampleSize:     totalSales,
			EnhanceSamples: totalEnhance,
			IsDefault:      isDefault,
		}
	}

	typeOptimalLevels := map[string]TypeOptimal{
		"normal":  calcTypeOptimal("normal"),
		"special": calcTypeOptimal("special"),
		"trash":   calcTypeOptimal("trash"),
	}

	// 타입별 레벨 효율 테이블 계산
	type TypeLevelEfficiency struct {
		Level              int     `json:"level"`
		AvgPrice           int     `json:"avg_price"`
		ExpectedTrials     float64 `json:"expected_trials"`
		ExpectedTimeSecond float64 `json:"expected_time_second"`
		SuccessProb        float64 `json:"success_prob"`
		GoldPerMinute      float64 `json:"gold_per_minute"`
		SampleSize         int     `json:"sample_size"`
		Recommendation     string  `json:"recommendation"`
	}

	calcTypeEfficiencies := func(itemType string) []TypeLevelEfficiency {
		var typeEffs []TypeLevelEfficiency
		bestLvl := 10
		bestGpm := 0.0

		// 해당 타입의 레벨별 샘플 수 계산
		getSampleSize := func(level int) int {
			if typeData, ok := typeLevelPrices[itemType]; ok {
				if entry, ok := typeData[level]; ok {
					return entry.count
				}
			}
			return 0
		}

		// 레벨 5-15 범위에서 분석
		for level := 5; level <= 15; level++ {
			price := getAvgPriceForType(itemType, level)
			timeSeconds := calcExpectedTimeForType(itemType, level)
			sampleSize := getSampleSize(level)

			// 성공 확률 계산 (타입별)
			successProb := 1.0
			expectedTrials := 0.0
			for lvl := 0; lvl < level; lvl++ {
				rate := getEnhanceRateForType(itemType, lvl)
				successProb *= rate
				if rate > 0 {
					expectedTrials += 1.0 / rate
				}
			}

			gpm := 0.0
			if timeSeconds > 0 {
				gpm = (float64(price) * successProb) / (timeSeconds / 60.0)
			}

			if gpm > bestGpm {
				bestGpm = gpm
				bestLvl = level
			}

			typeEffs = append(typeEffs, TypeLevelEfficiency{
				Level:              level,
				AvgPrice:           price,
				ExpectedTrials:     expectedTrials,
				ExpectedTimeSecond: timeSeconds,
				SuccessProb:        successProb * 100,
				GoldPerMinute:      gpm,
				SampleSize:         sampleSize,
				Recommendation:     "",
			})
		}

		// 최적 레벨에 추천 표시
		for i := range typeEffs {
			if typeEffs[i].Level == bestLvl {
				typeEffs[i].Recommendation = "optimal"
			}
		}

		return typeEffs
	}

	levelEfficienciesByType := map[string][]TypeLevelEfficiency{
		"normal":  calcTypeEfficiencies("normal"),
		"special": calcTypeEfficiencies("special"),
		"trash":   calcTypeEfficiencies("trash"),
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"optimal_level":              bestLevel,
		"optimal_gpm":                bestGPM,
		"level_efficiencies":         efficiencies,
		"by_type":                    typeOptimalLevels,
		"level_efficiencies_by_type": levelEfficienciesByType,
		"note":                       "gold_per_minute = (avg_price × success_prob) / (expected_time / 60)",
	})
}

// v3: 레벨별 강화 실측 통계
func handleEnhanceLevelDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type LevelEntry struct {
		Level       int     `json:"level"`
		Attempts    int     `json:"attempts"`
		Success     int     `json:"success"`
		Fail        int     `json:"fail"`
		Destroy     int     `json:"destroy"`
		SuccessRate float64 `json:"success_rate"`
		KeepRate    float64 `json:"keep_rate"`
		DestroyRate float64 `json:"destroy_rate"`
		Default     bool    `json:"is_default"` // 기본값 사용 여부
	}

	var levels []LevelEntry
	for _, def := range defaultEnhanceRates {
		entry := LevelEntry{
			Level:       def.Level,
			SuccessRate: def.SuccessRate,
			KeepRate:    def.KeepRate,
			DestroyRate: def.DestroyRate,
			Default:     true,
		}
		if detail, ok := stats.enhanceLevelDetail[def.Level]; ok && detail.Attempts > 0 {
			entry.Attempts = detail.Attempts
			entry.Success = detail.Success
			entry.Fail = detail.Fail
			entry.Destroy = detail.Destroy
			total := float64(detail.Attempts)
			entry.SuccessRate = float64(detail.Success) / total * 100
			entry.KeepRate = float64(detail.Fail) / total * 100
			entry.DestroyRate = float64(detail.Destroy) / total * 100
			entry.Default = detail.Attempts < minSampleSize
		}
		levels = append(levels, entry)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"min_sample_size": minSampleSize,
		"levels":          levels,
	})
}

func generateSignature(sessionID, period string) string {
	h := sha256.Sum256([]byte(sessionID + period + getAppSecret()))
	return hex.EncodeToString(h[:])[:16]
}

// validateTelemetryPayload 텔레메트리 페이로드 검증
func validateTelemetryPayload(p *TelemetryPayload) error {
	// 필수 필드 검증
	if p.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if len(p.SessionID) > maxSessionIDLen {
		return fmt.Errorf("session_id too long")
	}
	if len(p.AppVersion) > maxAppVersionLen {
		return fmt.Errorf("app_version too long")
	}
	if len(p.OSType) > maxOSTypeLen {
		return fmt.Errorf("os_type too long")
	}
	if len(p.Period) > maxPeriodLen {
		return fmt.Errorf("period too long")
	}

	// 스키마 버전 검증
	if p.SchemaVersion < 1 || p.SchemaVersion > 10 {
		return fmt.Errorf("invalid schema_version")
	}

	// 통계 값 범위 검증
	if err := validateStatValues(&p.Stats); err != nil {
		return err
	}

	// 맵 크기 검증
	if len(p.Stats.EnhanceByLevel) > maxMapEntries {
		return fmt.Errorf("enhance_by_level too many entries")
	}
	if len(p.Stats.SwordBattleStats) > maxMapEntries {
		return fmt.Errorf("sword_battle_stats too many entries")
	}
	if len(p.Stats.SpecialFoundByName) > maxMapEntries {
		return fmt.Errorf("special_found_by_name too many entries")
	}
	if len(p.Stats.UpsetStatsByDiff) > maxMapEntries {
		return fmt.Errorf("upset_stats_by_diff too many entries")
	}
	if len(p.Stats.SwordSaleStats) > maxMapEntries {
		return fmt.Errorf("sword_sale_stats too many entries")
	}
	if len(p.Stats.SwordEnhanceStats) > maxMapEntries {
		return fmt.Errorf("sword_enhance_stats too many entries")
	}
	if len(p.Stats.ItemFarmingStats) > maxMapEntries {
		return fmt.Errorf("item_farming_stats too many entries")
	}
	if len(p.Stats.EnhanceLevelDetail) > maxMapEntries {
		return fmt.Errorf("enhance_level_detail too many entries")
	}

	// 맵 키 길이 검증
	for name := range p.Stats.SwordBattleStats {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("sword name too long: %s", name)
		}
	}
	for name := range p.Stats.SpecialFoundByName {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("special name too long: %s", name)
		}
	}
	for name := range p.Stats.ItemFarmingStats {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("item name too long: %s", name)
		}
	}
	for name := range p.Stats.SwordEnhanceStats {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("enhance sword name too long: %s", name)
		}
	}

	return nil
}

// validateStatValues 통계 값 범위 검증 (음수 및 과도하게 큰 값 방지)
func validateStatValues(s *TelemetryStats) error {
	// 음수 검증
	if s.TotalCycles < 0 || s.SuccessfulCycles < 0 || s.FailedCycles < 0 {
		return fmt.Errorf("negative cycle values")
	}
	if s.TotalGoldMined < 0 || s.BattleGoldEarned < 0 {
		return fmt.Errorf("negative gold values")
	}
	if s.EnhanceAttempts < 0 || s.BattleCount < 0 || s.FarmingAttempts < 0 {
		return fmt.Errorf("negative attempt values")
	}

	// 최대값 검증
	if s.TotalCycles > maxStatValue || s.EnhanceAttempts > maxStatValue {
		return fmt.Errorf("stat value too large")
	}
	if s.BattleCount > maxStatValue || s.FarmingAttempts > maxStatValue {
		return fmt.Errorf("stat value too large")
	}

	// 레벨 범위 검증 (EnhanceByLevel)
	for level, count := range s.EnhanceByLevel {
		if level < 0 || level > 20 {
			return fmt.Errorf("invalid enhance level: %d", level)
		}
		if count < 0 || count > maxStatValue {
			return fmt.Errorf("invalid enhance count for level %d", level)
		}
	}

	// v3 값 검증
	if s.EnhanceCostTotal < 0 || s.BattleGoldLost < 0 {
		return fmt.Errorf("negative v3 gold values")
	}
	if s.CycleTimeTotal < 0 {
		return fmt.Errorf("negative cycle time")
	}
	for lvl, stat := range s.EnhanceLevelDetail {
		if lvl < 0 || lvl > 20 {
			return fmt.Errorf("invalid enhance level detail: %d", lvl)
		}
		if stat != nil && (stat.Attempts < 0 || stat.Success < 0 || stat.Fail < 0 || stat.Destroy < 0) {
			return fmt.Errorf("negative enhance level detail for level %d", lvl)
		}
	}

	// 역배 레벨차 검증 (1-20 허용)
	for diff, stat := range s.UpsetStatsByDiff {
		if diff < 1 || diff > 20 {
			return fmt.Errorf("invalid upset level diff: %d", diff)
		}
		if stat != nil && (stat.Attempts < 0 || stat.Wins < 0 || stat.GoldEarned < 0) {
			return fmt.Errorf("negative upset stats for diff %d", diff)
		}
	}

	return nil
}

// ========================
// SQLite 영구 저장소
// ========================

func initDB() error {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./sword-stats.db"
	}

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("DB 열기 실패: %v", err)
	}

	// WAL 모드 (동시 읽기/쓰기 성능 향상)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("WAL 설정 실패: %v", err)
	}

	// 테이블 생성
	tables := []string{
		`CREATE TABLE IF NOT EXISTS global_stats (
			id INTEGER PRIMARY KEY DEFAULT 1,
			enhance_attempts INTEGER DEFAULT 0,
			enhance_success INTEGER DEFAULT 0,
			enhance_fail INTEGER DEFAULT 0,
			enhance_destroy INTEGER DEFAULT 0,
			battle_count INTEGER DEFAULT 0,
			battle_wins INTEGER DEFAULT 0,
			upset_attempts INTEGER DEFAULT 0,
			upset_wins INTEGER DEFAULT 0,
			battle_gold INTEGER DEFAULT 0,
			farming_attempts INTEGER DEFAULT 0,
			special_found INTEGER DEFAULT 0,
			sales_count INTEGER DEFAULT 0,
			sales_total_gold INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS enhance_by_level (
			level INTEGER PRIMARY KEY,
			count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS sword_battle_stats (
			name TEXT PRIMARY KEY,
			battle_count INTEGER DEFAULT 0,
			battle_wins INTEGER DEFAULT 0,
			upset_attempts INTEGER DEFAULT 0,
			upset_wins INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS special_found_by_name (
			name TEXT PRIMARY KEY,
			count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS upset_stats_by_diff (
			level_diff INTEGER PRIMARY KEY,
			attempts INTEGER DEFAULT 0,
			wins INTEGER DEFAULT 0,
			gold_earned INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS sword_sale_stats (
			key TEXT PRIMARY KEY,
			total_price INTEGER DEFAULT 0,
			count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS sword_enhance_stats (
			name TEXT PRIMARY KEY,
			attempts INTEGER DEFAULT 0,
			success INTEGER DEFAULT 0,
			fail INTEGER DEFAULT 0,
			destroy INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS item_farming_stats (
			name TEXT PRIMARY KEY,
			total_count INTEGER DEFAULT 0,
			special_count INTEGER DEFAULT 0,
			normal_count INTEGER DEFAULT 0,
			trash_count INTEGER DEFAULT 0
		)`,
		// v3 테이블
		`CREATE TABLE IF NOT EXISTS enhance_level_detail (
			level INTEGER PRIMARY KEY,
			attempts INTEGER DEFAULT 0,
			success INTEGER DEFAULT 0,
			fail INTEGER DEFAULT 0,
			destroy INTEGER DEFAULT 0
		)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("테이블 생성 실패: %v", err)
		}
	}

	// v3 마이그레이션: global_stats에 새 컬럼 추가 (이미 있으면 무시)
	migrations := []string{
		"ALTER TABLE global_stats ADD COLUMN enhance_cost_total INTEGER DEFAULT 0",
		"ALTER TABLE global_stats ADD COLUMN cycle_time_total REAL DEFAULT 0",
		"ALTER TABLE global_stats ADD COLUMN battle_gold_lost INTEGER DEFAULT 0",
		"ALTER TABLE item_farming_stats ADD COLUMN trash_count INTEGER DEFAULT 0",
	}
	for _, m := range migrations {
		db.Exec(m) // 이미 존재하면 에러 → 무시
	}

	// global_stats 초기 행 (없으면 생성)
	db.Exec("INSERT OR IGNORE INTO global_stats (id) VALUES (1)")

	log.Printf("📦 SQLite DB 초기화 완료: %s", dbPath)
	return nil
}

func loadFromDB() error {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	// global_stats 로드 (v3 컬럼 포함)
	row := db.QueryRow("SELECT enhance_attempts, enhance_success, enhance_fail, enhance_destroy, battle_count, battle_wins, upset_attempts, upset_wins, battle_gold, farming_attempts, special_found, sales_count, sales_total_gold, COALESCE(enhance_cost_total,0), COALESCE(cycle_time_total,0), COALESCE(battle_gold_lost,0) FROM global_stats WHERE id=1")
	if err := row.Scan(
		&stats.enhanceAttempts, &stats.enhanceSuccess, &stats.enhanceFail, &stats.enhanceDestroy,
		&stats.battleCount, &stats.battleWins, &stats.upsetAttempts, &stats.upsetWins, &stats.battleGold,
		&stats.farmingAttempts, &stats.specialFound, &stats.salesCount, &stats.salesTotalGold,
		&stats.enhanceCostTotal, &stats.cycleTimeTotal, &stats.battleGoldLost,
	); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("global_stats 로드 실패: %v", err)
	}

	// enhance_by_level 로드
	rows, err := db.Query("SELECT level, count FROM enhance_by_level")
	if err != nil {
		return fmt.Errorf("enhance_by_level 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var level, count int
		if err := rows.Scan(&level, &count); err == nil {
			stats.enhanceByLevel[level] = count
		}
	}

	// sword_battle_stats 로드
	rows, err = db.Query("SELECT name, battle_count, battle_wins, upset_attempts, upset_wins FROM sword_battle_stats")
	if err != nil {
		return fmt.Errorf("sword_battle_stats 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		s := &SwordBattleStat{}
		if err := rows.Scan(&name, &s.BattleCount, &s.BattleWins, &s.UpsetAttempts, &s.UpsetWins); err == nil {
			stats.swordBattleStats[name] = s
		}
	}

	// special_found_by_name 로드
	rows, err = db.Query("SELECT name, count FROM special_found_by_name")
	if err != nil {
		return fmt.Errorf("special_found_by_name 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err == nil {
			stats.specialFoundByName[name] = count
		}
	}

	// upset_stats_by_diff 로드
	rows, err = db.Query("SELECT level_diff, attempts, wins, gold_earned FROM upset_stats_by_diff")
	if err != nil {
		return fmt.Errorf("upset_stats_by_diff 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var diff int
		s := &UpsetStat{}
		if err := rows.Scan(&diff, &s.Attempts, &s.Wins, &s.GoldEarned); err == nil {
			stats.upsetStatsByDiff[diff] = s
		}
	}

	// sword_sale_stats 로드
	rows, err = db.Query("SELECT key, total_price, count FROM sword_sale_stats")
	if err != nil {
		return fmt.Errorf("sword_sale_stats 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		s := &SwordSaleStat{}
		if err := rows.Scan(&key, &s.TotalPrice, &s.Count); err == nil {
			stats.swordSaleStats[key] = s
		}
	}

	// sword_enhance_stats 로드
	rows, err = db.Query("SELECT name, attempts, success, fail, destroy FROM sword_enhance_stats")
	if err != nil {
		return fmt.Errorf("sword_enhance_stats 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		s := &SwordEnhanceStat{}
		if err := rows.Scan(&name, &s.Attempts, &s.Success, &s.Fail, &s.Destroy); err == nil {
			stats.swordEnhanceStats[name] = s
		}
	}

	// item_farming_stats 로드
	rows, err = db.Query("SELECT name, total_count, special_count, normal_count, COALESCE(trash_count,0) FROM item_farming_stats")
	if err != nil {
		return fmt.Errorf("item_farming_stats 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		s := &ItemFarmingStat{}
		if err := rows.Scan(&name, &s.TotalCount, &s.SpecialCount, &s.NormalCount, &s.TrashCount); err == nil {
			stats.itemFarmingStats[name] = s
		}
	}

	// v3: enhance_level_detail 로드
	rows, err = db.Query("SELECT level, attempts, success, fail, destroy FROM enhance_level_detail")
	if err != nil {
		return fmt.Errorf("enhance_level_detail 로드 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var level int
		s := &EnhanceLevelStat{}
		if err := rows.Scan(&level, &s.Attempts, &s.Success, &s.Fail, &s.Destroy); err == nil {
			stats.enhanceLevelDetail[level] = s
		}
	}

	log.Printf("📦 DB에서 통계 로드 완료")
	return nil
}

func saveToDB() {
	stats.mu.RLock()
	defer stats.mu.RUnlock()

	tx, err := db.Begin()
	if err != nil {
		log.Printf("[DB] 트랜잭션 시작 실패: %v", err)
		return
	}
	defer tx.Rollback()

	// global_stats 저장 (v3 컬럼 포함)
	tx.Exec(`UPDATE global_stats SET
		enhance_attempts=?, enhance_success=?, enhance_fail=?, enhance_destroy=?,
		battle_count=?, battle_wins=?, upset_attempts=?, upset_wins=?, battle_gold=?,
		farming_attempts=?, special_found=?, sales_count=?, sales_total_gold=?,
		enhance_cost_total=?, cycle_time_total=?, battle_gold_lost=?
		WHERE id=1`,
		stats.enhanceAttempts, stats.enhanceSuccess, stats.enhanceFail, stats.enhanceDestroy,
		stats.battleCount, stats.battleWins, stats.upsetAttempts, stats.upsetWins, stats.battleGold,
		stats.farmingAttempts, stats.specialFound, stats.salesCount, stats.salesTotalGold,
		stats.enhanceCostTotal, stats.cycleTimeTotal, stats.battleGoldLost,
	)

	// enhance_by_level 저장
	for level, count := range stats.enhanceByLevel {
		tx.Exec("INSERT OR REPLACE INTO enhance_by_level (level, count) VALUES (?, ?)", level, count)
	}

	// sword_battle_stats 저장
	for name, s := range stats.swordBattleStats {
		tx.Exec("INSERT OR REPLACE INTO sword_battle_stats (name, battle_count, battle_wins, upset_attempts, upset_wins) VALUES (?, ?, ?, ?, ?)",
			name, s.BattleCount, s.BattleWins, s.UpsetAttempts, s.UpsetWins)
	}

	// special_found_by_name 저장
	for name, count := range stats.specialFoundByName {
		tx.Exec("INSERT OR REPLACE INTO special_found_by_name (name, count) VALUES (?, ?)", name, count)
	}

	// upset_stats_by_diff 저장
	for diff, s := range stats.upsetStatsByDiff {
		tx.Exec("INSERT OR REPLACE INTO upset_stats_by_diff (level_diff, attempts, wins, gold_earned) VALUES (?, ?, ?, ?)",
			diff, s.Attempts, s.Wins, s.GoldEarned)
	}

	// sword_sale_stats 저장
	for key, s := range stats.swordSaleStats {
		tx.Exec("INSERT OR REPLACE INTO sword_sale_stats (key, total_price, count) VALUES (?, ?, ?)",
			key, s.TotalPrice, s.Count)
	}

	// sword_enhance_stats 저장
	for name, s := range stats.swordEnhanceStats {
		tx.Exec("INSERT OR REPLACE INTO sword_enhance_stats (name, attempts, success, fail, destroy) VALUES (?, ?, ?, ?, ?)",
			name, s.Attempts, s.Success, s.Fail, s.Destroy)
	}

	// item_farming_stats 저장
	for name, s := range stats.itemFarmingStats {
		tx.Exec("INSERT OR REPLACE INTO item_farming_stats (name, total_count, special_count, normal_count, trash_count) VALUES (?, ?, ?, ?, ?)",
			name, s.TotalCount, s.SpecialCount, s.NormalCount, s.TrashCount)
	}

	// v3: enhance_level_detail 저장
	for lvl, s := range stats.enhanceLevelDetail {
		tx.Exec("INSERT OR REPLACE INTO enhance_level_detail (level, attempts, success, fail, destroy) VALUES (?, ?, ?, ?, ?)",
			lvl, s.Attempts, s.Success, s.Fail, s.Destroy)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] 커밋 실패: %v", err)
	}
}

func main() {
	// SQLite 초기화
	if err := initDB(); err != nil {
		log.Printf("⚠️ DB 초기화 실패 (인메모리 모드로 동작): %v", err)
	} else {
		defer db.Close()
		if err := loadFromDB(); err != nil {
			log.Printf("⚠️ DB 로드 실패: %v", err)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 라우팅
	http.HandleFunc("/", handleHealth)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/game-data", handleGameData)
	http.HandleFunc("/api/telemetry", handleTelemetry)
	http.HandleFunc("/api/stats/detailed", handleStatsDetailed)
	// v2 엔드포인트
	http.HandleFunc("/api/stats/swords", handleSwordStats)
	http.HandleFunc("/api/stats/special", handleSpecialStats)
	http.HandleFunc("/api/stats/upset", handleUpsetStats)
	http.HandleFunc("/api/stats/items", handleItemStats)
	http.HandleFunc("/api/stats/enhance", handleEnhanceStats)
	http.HandleFunc("/api/stats/sales", handleSaleStats)
	http.HandleFunc("/api/strategy/optimal-sell-point", handleOptimalSellPoint)
	// v3 엔드포인트
	http.HandleFunc("/api/stats/enhance-levels", handleEnhanceLevelDetail)

	log.Printf("🚀 Sword API 서버 시작 (포트: %s)", port)
	log.Printf("   /api/game-data - 게임 데이터 조회 (실측 확률 반영)")
	log.Printf("   /api/telemetry - 텔레메트리 수신 (v3 스키마)")
	log.Printf("   /api/stats/detailed - 커뮤니티 통계")
	log.Printf("   /api/stats/swords - 검 종류별 승률 (v2)")
	log.Printf("   /api/stats/special - 특수 검 출현 확률 (v2)")
	log.Printf("   /api/stats/upset - 역배 실측 승률 (v2)")
	log.Printf("   /api/stats/items - 아이템 파밍 통계 (v2)")
	log.Printf("   /api/stats/enhance - 검 종류별 강화 성공률 (v2)")
	log.Printf("   /api/stats/sales - 검+레벨별 판매 통계 (v2)")
	log.Printf("   /api/stats/enhance-levels - 레벨별 강화 확률 (v3)")
	log.Printf("   /api/strategy/optimal-sell-point - 최적 판매 시점")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
