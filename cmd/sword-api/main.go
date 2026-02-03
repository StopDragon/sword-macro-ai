package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

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
	HiddenFound      int         `json:"hidden_found"`
	TrashFound       int         `json:"trash_found"`

	// === v2 새로 추가 ===
	SwordBattleStats  map[string]*SwordBattleStat  `json:"sword_battle_stats,omitempty"`
	HiddenFoundByName map[string]int               `json:"hidden_found_by_name,omitempty"`
	UpsetStatsByDiff  map[int]*UpsetStat           `json:"upset_stats_by_diff,omitempty"`
	SwordSaleStats    map[string]*SwordSaleStat    `json:"sword_sale_stats,omitempty"`
	ItemFarmingStats  map[string]*ItemFarmingStat  `json:"item_farming_stats,omitempty"`
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

// ItemFarmingStat 아이템별 파밍 통계
type ItemFarmingStat struct {
	TotalCount  int `json:"total_count"`
	HiddenCount int `json:"hidden_count"`
	NormalCount int `json:"normal_count"`
}

type TelemetryPayload struct {
	SchemaVersion int            `json:"schema_version"`
	AppVersion    string         `json:"app_version"`
	OSType        string         `json:"os_type"`
	SessionID     string         `json:"session_id"`
	Period        string         `json:"period"`
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
	hiddenFound     int
	salesCount      int
	salesTotalGold  int

	// === v2 통계 ===
	swordBattleStats  map[string]*SwordBattleStat
	hiddenFoundByName map[string]int
	upsetStatsByDiff  map[int]*UpsetStat
	swordSaleStats    map[string]*SwordSaleStat
	itemFarmingStats  map[string]*ItemFarmingStat
}

var stats = &StatsStore{
	enhanceByLevel:    make(map[int]int),
	swordBattleStats:  make(map[string]*SwordBattleStat),
	hiddenFoundByName: make(map[string]int),
	upsetStatsByDiff:  make(map[int]*UpsetStat),
	swordSaleStats:    make(map[string]*SwordSaleStat),
	itemFarmingStats:  make(map[string]*ItemFarmingStat),
}

// ========================
// 게임 데이터 (DB에서 가져오는 것처럼 구조화)
// ========================

func getGameData() GameData {
	return GameData{
		EnhanceRates: []EnhanceRate{
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
		},
		SwordPrices: []SwordPrice{
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
		},
		BattleRewards: []BattleReward{
			{LevelDiff: 1, WinRate: 35.0, MinReward: 500, MaxReward: 1500, AvgReward: 1000},
			{LevelDiff: 2, WinRate: 20.0, MinReward: 1500, MaxReward: 4000, AvgReward: 2750},
			{LevelDiff: 3, WinRate: 10.0, MinReward: 4000, MaxReward: 10000, AvgReward: 7000},
		},
		UpdatedAt: time.Now().Format(time.RFC3339),
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
	stats.hiddenFound += payload.Stats.HiddenFound
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

		// 히든 이름별 통계
		for name, cnt := range payload.Stats.HiddenFoundByName {
			stats.hiddenFoundByName[name] += cnt
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

		// 아이템 파밍 통계
		for name, stat := range payload.Stats.ItemFarmingStats {
			if stats.itemFarmingStats[name] == nil {
				stats.itemFarmingStats[name] = &ItemFarmingStat{}
			}
			stats.itemFarmingStats[name].TotalCount += stat.TotalCount
			stats.itemFarmingStats[name].HiddenCount += stat.HiddenCount
			stats.itemFarmingStats[name].NormalCount += stat.NormalCount
		}
	}
	stats.mu.Unlock()

	log.Printf("[텔레메트리] 세션=%s 버전=%s OS=%s", payload.SessionID[:8], payload.AppVersion, payload.OSType)

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
	hiddenRate := "0%"
	if stats.farmingAttempts > 0 {
		hiddenRate = fmt.Sprintf("%.2f%%", float64(stats.hiddenFound)/float64(stats.farmingAttempts)*100)
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
			"히든_확률": hiddenRate,
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

// 히든 검 출현 확률
func handleHiddenStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	type HiddenEntry struct {
		Name  string  `json:"name"`
		Count int     `json:"count"`
		Rate  float64 `json:"rate"`
	}

	var hidden []HiddenEntry
	for name, cnt := range stats.hiddenFoundByName {
		rate := 0.0
		if stats.farmingAttempts > 0 {
			rate = float64(cnt) / float64(stats.farmingAttempts) * 100
		}
		hidden = append(hidden, HiddenEntry{
			Name:  name,
			Count: cnt,
			Rate:  rate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_farming": stats.farmingAttempts,
		"hidden":        hidden,
	})
}

// 역배 실측 승률
func handleUpsetStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats.mu.RLock()
	defer stats.mu.RUnlock()

	// 이론 승률
	theoryRates := map[int]float64{
		1: 35.0,
		2: 20.0,
		3: 10.0,
	}

	type DiffStat struct {
		Attempts   int     `json:"attempts"`
		Wins       int     `json:"wins"`
		WinRate    float64 `json:"win_rate"`
		Theory     float64 `json:"theory"`
		GoldEarned int     `json:"gold_earned"`
	}

	byDiff := make(map[string]DiffStat)
	for diff := 1; diff <= 3; diff++ {
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
		Name        string  `json:"name"`
		TotalCount  int     `json:"total_count"`
		HiddenCount int     `json:"hidden_count"`
		NormalCount int     `json:"normal_count"`
		HiddenRate  float64 `json:"hidden_rate"`
	}

	var items []ItemEntry
	for name, stat := range stats.itemFarmingStats {
		hiddenRate := 0.0
		if stat.TotalCount > 0 {
			hiddenRate = float64(stat.HiddenCount) / float64(stat.TotalCount) * 100
		}
		items = append(items, ItemEntry{
			Name:        name,
			TotalCount:  stat.TotalCount,
			HiddenCount: stat.HiddenCount,
			NormalCount: stat.NormalCount,
			HiddenRate:  hiddenRate,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_farming": stats.farmingAttempts,
		"items":         items,
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
	if len(p.Stats.HiddenFoundByName) > maxMapEntries {
		return fmt.Errorf("hidden_found_by_name too many entries")
	}
	if len(p.Stats.UpsetStatsByDiff) > maxMapEntries {
		return fmt.Errorf("upset_stats_by_diff too many entries")
	}
	if len(p.Stats.SwordSaleStats) > maxMapEntries {
		return fmt.Errorf("sword_sale_stats too many entries")
	}
	if len(p.Stats.ItemFarmingStats) > maxMapEntries {
		return fmt.Errorf("item_farming_stats too many entries")
	}

	// 맵 키 길이 검증
	for name := range p.Stats.SwordBattleStats {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("sword name too long: %s", name)
		}
	}
	for name := range p.Stats.HiddenFoundByName {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("hidden name too long: %s", name)
		}
	}
	for name := range p.Stats.ItemFarmingStats {
		if len(name) > maxSwordNameLen {
			return fmt.Errorf("item name too long: %s", name)
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

	// 역배 레벨차 검증
	for diff, stat := range s.UpsetStatsByDiff {
		if diff < 1 || diff > 10 {
			return fmt.Errorf("invalid upset level diff: %d", diff)
		}
		if stat != nil && (stat.Attempts < 0 || stat.Wins < 0 || stat.GoldEarned < 0) {
			return fmt.Errorf("negative upset stats for diff %d", diff)
		}
	}

	return nil
}

func main() {
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
	http.HandleFunc("/api/stats/hidden", handleHiddenStats)
	http.HandleFunc("/api/stats/upset", handleUpsetStats)
	http.HandleFunc("/api/stats/items", handleItemStats)

	log.Printf("🚀 Sword API 서버 시작 (포트: %s)", port)
	log.Printf("   /api/game-data - 게임 데이터 조회")
	log.Printf("   /api/telemetry - 텔레메트리 수신")
	log.Printf("   /api/stats/detailed - 커뮤니티 통계")
	log.Printf("   /api/stats/swords - 검 종류별 승률 (v2)")
	log.Printf("   /api/stats/hidden - 히든 검 출현 확률 (v2)")
	log.Printf("   /api/stats/upset - 역배 실측 승률 (v2)")
	log.Printf("   /api/stats/items - 아이템 파밍 통계 (v2)")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
