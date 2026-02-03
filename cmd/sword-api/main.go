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
	appSecret = "sw0rd-m4cr0-2026-s3cr3t-k3y"
)

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
}

var stats = &StatsStore{
	enhanceByLevel: make(map[int]int),
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

	// 서명 검증
	expectedSig := generateSignature(payload.SessionID, payload.Period)
	if signature != expectedSig {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// 통계 업데이트
	stats.mu.Lock()
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

func generateSignature(sessionID, period string) string {
	h := sha256.Sum256([]byte(sessionID + period + appSecret))
	return hex.EncodeToString(h[:])[:16]
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

	log.Printf("🚀 Sword API 서버 시작 (포트: %s)", port)
	log.Printf("   /api/game-data - 게임 데이터 조회")
	log.Printf("   /api/telemetry - 텔레메트리 수신")
	log.Printf("   /api/stats/detailed - 커뮤니티 통계")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
