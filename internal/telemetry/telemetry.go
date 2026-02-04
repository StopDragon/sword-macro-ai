package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/StopDragon/sword-macro-ai/internal/logger"
	"github.com/google/uuid"
)

const (
	endpoint         = "https://sword-ai.stopdragon.kr/api/telemetry"
	stateFile        = ".telemetry_state.json"
	schemaVer        = 2 // v2: 검 종류별 통계, 특수 이름별 통계, 세션 통계 추가
	sendTimeout      = 5 * time.Second
	sendInterval     = 3 * time.Minute
	defaultAppSecret = "sw0rd-m4cr0-2026-s3cr3t-k3y" // 환경변수 없을 때 기본값
	appSecretEnvVar  = "SWORD_APP_SECRET"
)

// getAppSecret 앱 시크릿 조회 (환경변수 우선)
func getAppSecret() string {
	if secret := os.Getenv(appSecretEnvVar); secret != "" {
		return secret
	}
	return defaultAppSecret
}

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
	Attempts int `json:"attempts"` // 강화 시도
	Success  int `json:"success"`  // 성공
	Fail     int `json:"fail"`     // 실패 (유지)
	Destroy  int `json:"destroy"`  // 파괴
}

// SessionStats 세션 통계
type SessionStats struct {
	StartingGold int `json:"starting_gold"`
	EndingGold   int `json:"ending_gold"`
	PeakGold     int `json:"peak_gold"`
	LowestGold   int `json:"lowest_gold"`
}

// ItemFarmingStat 아이템별 파밍 통계
type ItemFarmingStat struct {
	TotalCount   int `json:"total_count"`   // 총 획득 횟수
	SpecialCount int `json:"special_count"` // 특수로 획득한 횟수
	NormalCount  int `json:"normal_count"`  // 일반으로 획득한 횟수
	TrashCount   int `json:"trash_count"`   // 쓰레기로 획득한 횟수
}

// Stats 수집 통계
type Stats struct {
	// 기본 통계
	TotalCycles      int `json:"total_cycles"`
	SuccessfulCycles int `json:"successful_cycles"`
	FailedCycles     int `json:"failed_cycles"`
	TotalGoldMined   int `json:"total_gold_mined"`
	TotalSwordsFound int `json:"total_swords_found"`
	SessionDuration  int `json:"session_duration_sec"`

	// 강화 통계
	EnhanceAttempts int         `json:"enhance_attempts"`
	EnhanceSuccess  int         `json:"enhance_success"`
	EnhanceFail     int         `json:"enhance_fail"`
	EnhanceDestroy  int         `json:"enhance_destroy"`
	EnhanceByLevel  map[int]int `json:"enhance_by_level,omitempty"` // level -> success count

	// 배틀 통계
	BattleCount      int `json:"battle_count"`
	BattleWins       int `json:"battle_wins"`
	BattleLosses     int `json:"battle_losses"`
	BattleGoldEarned int `json:"battle_gold_earned"`
	UpsetWins        int `json:"upset_wins"`     // 역배 승리
	UpsetAttempts    int `json:"upset_attempts"` // 역배 시도

	// 판매 통계
	SalesCount     int `json:"sales_count"`
	SalesTotalGold int `json:"sales_total_gold"`
	SalesMaxPrice  int `json:"sales_max_price"`

	// 파밍 통계
	FarmingAttempts int `json:"farming_attempts"`
	SpecialFound    int `json:"special_found"` // 특수 아이템 발견 횟수
	TrashFound      int `json:"trash_found"`   // 쓰레기 아이템 발견 횟수

	// === v2 새로 추가 ===

	// 검 종류별 배틀 통계: "불꽃검" -> SwordBattleStat
	SwordBattleStats map[string]*SwordBattleStat `json:"sword_battle_stats,omitempty"`

	// 특수 아이템 발견 통계: "용검" -> 3
	SpecialFoundByName map[string]int `json:"special_found_by_name,omitempty"`

	// 모든 아이템 파밍 통계: "불꽃검" -> {count: 5, special: 1}
	ItemFarmingStats map[string]*ItemFarmingStat `json:"item_farming_stats,omitempty"`

	// 레벨차별 역배 통계: 1 -> UpsetStat, 2 -> UpsetStat, 3 -> UpsetStat
	UpsetStatsByDiff map[int]*UpsetStat `json:"upset_stats_by_diff,omitempty"`

	// 검+레벨별 판매 통계: "불꽃검_10" -> SwordSaleStat
	SwordSaleStats map[string]*SwordSaleStat `json:"sword_sale_stats,omitempty"`

	// 검 종류별 강화 통계: "불꽃검" -> SwordEnhanceStat
	SwordEnhanceStats map[string]*SwordEnhanceStat `json:"sword_enhance_stats,omitempty"`

	// 세션 통계
	Session *SessionStats `json:"session,omitempty"`
}

// Payload 서버 전송 데이터
type Payload struct {
	SchemaVersion int    `json:"schema_version"`
	AppVersion    string `json:"app_version"`
	OSType        string `json:"os_type"`
	SessionID     string `json:"session_id"`
	Period        string `json:"period"`
	Stats         Stats  `json:"stats"`
}

// state 내부 상태 (파일 저장용)
type state struct {
	Enabled      bool   `json:"enabled"`
	SessionID    string `json:"session_id"`
	LastSentTime int64  `json:"last_sent_time"`
	Stats        Stats  `json:"stats"`
	SessionStart int64  `json:"session_start"`
}

// Telemetry 텔레메트리 클라이언트
type Telemetry struct {
	mu           sync.Mutex
	enabled      bool
	sessionID    string
	appVersion   string
	stats        Stats
	sessionStart time.Time
	lastSentTime time.Time
	statePath    string
}

// New 텔레메트리 인스턴스 생성
func New(appVersion string) *Telemetry {
	t := &Telemetry{
		appVersion:   appVersion,
		sessionStart: time.Now(),
		statePath:    getStatePath(),
	}
	t.loadState()
	return t
}

// SetEnabled 텔레메트리 활성화/비활성화
func (t *Telemetry) SetEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = enabled
	t.saveState()
}

// IsEnabled 텔레메트리 활성화 여부
func (t *Telemetry) IsEnabled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.enabled
}

// RecordCycle 사이클 기록
func (t *Telemetry) RecordCycle(success bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}
	t.stats.TotalCycles++
	if success {
		t.stats.SuccessfulCycles++
	} else {
		t.stats.FailedCycles++
	}
}

// RecordGold 금광 채굴 기록
func (t *Telemetry) RecordGold(amount int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}
	t.stats.TotalGoldMined += amount
}

// RecordSword 검 획득 기록
func (t *Telemetry) RecordSword() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}
	t.stats.TotalSwordsFound++
}

// RecordEnhance 강화 결과 기록
func (t *Telemetry) RecordEnhance(level int, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	t.stats.EnhanceAttempts++

	switch result {
	case "success":
		t.stats.EnhanceSuccess++
		if t.stats.EnhanceByLevel == nil {
			t.stats.EnhanceByLevel = make(map[int]int)
		}
		t.stats.EnhanceByLevel[level]++
	case "fail":
		t.stats.EnhanceFail++
	case "destroy":
		t.stats.EnhanceDestroy++
	}
}

// RecordEnhanceWithSword 검 종류 포함 강화 기록
func (t *Telemetry) RecordEnhanceWithSword(swordName string, level int, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	// 기존 강화 통계 업데이트
	t.stats.EnhanceAttempts++

	switch result {
	case "success":
		t.stats.EnhanceSuccess++
		if t.stats.EnhanceByLevel == nil {
			t.stats.EnhanceByLevel = make(map[int]int)
		}
		t.stats.EnhanceByLevel[level]++
	case "fail", "hold":
		t.stats.EnhanceFail++
	case "destroy":
		t.stats.EnhanceDestroy++
	}

	// 검 종류별 강화 통계 (v2)
	if swordName != "" {
		if t.stats.SwordEnhanceStats == nil {
			t.stats.SwordEnhanceStats = make(map[string]*SwordEnhanceStat)
		}
		if t.stats.SwordEnhanceStats[swordName] == nil {
			t.stats.SwordEnhanceStats[swordName] = &SwordEnhanceStat{}
		}
		stat := t.stats.SwordEnhanceStats[swordName]
		stat.Attempts++

		switch result {
		case "success":
			stat.Success++
		case "fail", "hold":
			stat.Fail++
		case "destroy":
			stat.Destroy++
		}
	}
}

// RecordBattle 배틀 결과 기록
func (t *Telemetry) RecordBattle(myLevel, oppLevel int, won bool, goldEarned int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	t.stats.BattleCount++
	isUpset := oppLevel > myLevel

	if isUpset {
		t.stats.UpsetAttempts++
	}

	if won {
		t.stats.BattleWins++
		t.stats.BattleGoldEarned += goldEarned
		if isUpset {
			t.stats.UpsetWins++
		}
	} else {
		t.stats.BattleLosses++
	}
}

// RecordSale 판매 기록
func (t *Telemetry) RecordSale(level int, price int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	t.stats.SalesCount++
	t.stats.SalesTotalGold += price
	if price > t.stats.SalesMaxPrice {
		t.stats.SalesMaxPrice = price
	}
}

// RecordFarming 파밍 결과 기록
func (t *Telemetry) RecordFarming(isSpecial bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	t.stats.FarmingAttempts++
	if isSpecial {
		t.stats.SpecialFound++
	} else {
		t.stats.TrashFound++
	}
}

// === v2 새로운 Record 함수들 ===

// RecordBattleWithSword 검 종류 포함 배틀 기록
func (t *Telemetry) RecordBattleWithSword(swordName string, myLevel, oppLevel int, won bool, goldEarned int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	// 기존 배틀 통계 업데이트
	t.stats.BattleCount++
	levelDiff := oppLevel - myLevel
	isUpset := levelDiff > 0

	if isUpset {
		t.stats.UpsetAttempts++
	}

	if won {
		t.stats.BattleWins++
		t.stats.BattleGoldEarned += goldEarned
		if isUpset {
			t.stats.UpsetWins++
		}
	} else {
		t.stats.BattleLosses++
	}

	// 검 종류별 배틀 통계 (v2)
	if swordName != "" {
		if t.stats.SwordBattleStats == nil {
			t.stats.SwordBattleStats = make(map[string]*SwordBattleStat)
		}
		if t.stats.SwordBattleStats[swordName] == nil {
			t.stats.SwordBattleStats[swordName] = &SwordBattleStat{}
		}
		stat := t.stats.SwordBattleStats[swordName]
		stat.BattleCount++
		if isUpset {
			stat.UpsetAttempts++
		}
		if won {
			stat.BattleWins++
			if isUpset {
				stat.UpsetWins++
			}
		}
	}

	// 레벨차별 역배 통계 (v2) - 1-20 레벨 차이 지원
	if isUpset && levelDiff <= 20 {
		if t.stats.UpsetStatsByDiff == nil {
			t.stats.UpsetStatsByDiff = make(map[int]*UpsetStat)
		}
		if t.stats.UpsetStatsByDiff[levelDiff] == nil {
			t.stats.UpsetStatsByDiff[levelDiff] = &UpsetStat{}
		}
		stat := t.stats.UpsetStatsByDiff[levelDiff]
		stat.Attempts++
		if won {
			stat.Wins++
			stat.GoldEarned += goldEarned
		}
	}
}

// RecordSpecialWithName 특수 아이템 이름 포함 기록
func (t *Telemetry) RecordSpecialWithName(swordName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	// 기존 통계 업데이트
	t.stats.FarmingAttempts++
	t.stats.SpecialFound++

	// 특수 아이템 이름별 통계 (v2)
	if swordName != "" {
		if t.stats.SpecialFoundByName == nil {
			t.stats.SpecialFoundByName = make(map[string]int)
		}
		t.stats.SpecialFoundByName[swordName]++
	}
}

// RecordFarmingWithItem 아이템 이름과 타입 포함 파밍 기록
// itemType: "special"(특수), "normal"(일반), "trash"(쓰레기)
func (t *Telemetry) RecordFarmingWithItem(itemName string, itemType string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	// 기존 통계 업데이트
	t.stats.FarmingAttempts++
	if itemType == "special" {
		t.stats.SpecialFound++
	} else if itemType == "trash" || itemType == "normal" {
		t.stats.TrashFound++
	}

	// 아이템별 파밍 통계 (v2)
	if itemName != "" {
		// 특수 이름별 통계
		if itemType == "special" {
			if t.stats.SpecialFoundByName == nil {
				t.stats.SpecialFoundByName = make(map[string]int)
			}
			t.stats.SpecialFoundByName[itemName]++
		}

		// 전체 아이템 통계
		if t.stats.ItemFarmingStats == nil {
			t.stats.ItemFarmingStats = make(map[string]*ItemFarmingStat)
		}
		if t.stats.ItemFarmingStats[itemName] == nil {
			t.stats.ItemFarmingStats[itemName] = &ItemFarmingStat{}
		}
		stat := t.stats.ItemFarmingStats[itemName]
		stat.TotalCount++
		if itemType == "special" {
			stat.SpecialCount++
		} else if itemType == "trash" {
			stat.TrashCount++
		} else {
			stat.NormalCount++
		}
	}
}

// RecordSaleWithSword 검 종류 포함 판매 기록
func (t *Telemetry) RecordSaleWithSword(swordName string, level int, price int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	// 기존 판매 통계 업데이트
	t.stats.SalesCount++
	t.stats.SalesTotalGold += price
	if price > t.stats.SalesMaxPrice {
		t.stats.SalesMaxPrice = price
	}

	// 검+레벨별 판매 통계 (v2)
	if swordName != "" {
		if t.stats.SwordSaleStats == nil {
			t.stats.SwordSaleStats = make(map[string]*SwordSaleStat)
		}
		key := fmt.Sprintf("%s_%d", swordName, level)
		if t.stats.SwordSaleStats[key] == nil {
			t.stats.SwordSaleStats[key] = &SwordSaleStat{}
		}
		stat := t.stats.SwordSaleStats[key]
		stat.Count++
		stat.TotalPrice += price
	}
}

// InitSession 세션 시작 시 초기화
func (t *Telemetry) InitSession(startingGold int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	t.stats.Session = &SessionStats{
		StartingGold: startingGold,
		EndingGold:   startingGold,
		PeakGold:     startingGold,
		LowestGold:   startingGold,
	}
}

// RecordGoldChange 골드 변화 기록 (세션 통계용)
func (t *Telemetry) RecordGoldChange(currentGold int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled || t.stats.Session == nil {
		return
	}

	t.stats.Session.EndingGold = currentGold
	if currentGold > t.stats.Session.PeakGold {
		t.stats.Session.PeakGold = currentGold
	}
	if currentGold < t.stats.Session.LowestGold {
		t.stats.Session.LowestGold = currentGold
	}
}

// RecordProfile 프로필 정보 기록 (세션 시작 시)
// 주의: username은 로컬 디버그 로깅에만 사용되며, 서버로 전송되지 않음 (개인정보 보호)
func (t *Telemetry) RecordProfile(username string, level int, gold int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	// 세션 초기화
	if t.stats.Session == nil {
		t.stats.Session = &SessionStats{
			StartingGold: gold,
			EndingGold:   gold,
			PeakGold:     gold,
			LowestGold:   gold,
		}
	} else {
		t.stats.Session.StartingGold = gold
		t.stats.Session.EndingGold = gold
		t.stats.Session.PeakGold = gold
		t.stats.Session.LowestGold = gold
	}

	// 로그
	logger.Debug("프로필 기록: %s, +%d, %dG", username, level, gold)
}

// TrySend 3분 간격으로 서버에 전송 시도
func (t *Telemetry) TrySend() {
	t.mu.Lock()

	if !t.enabled {
		t.mu.Unlock()
		return
	}

	// 3분 경과 확인
	if time.Since(t.lastSentTime) < sendInterval {
		t.mu.Unlock()
		return
	}

	// 전송할 데이터가 없으면 스킵
	if t.stats.TotalCycles == 0 && t.stats.TotalSwordsFound == 0 &&
		t.stats.BattleCount == 0 && t.stats.SalesCount == 0 && t.stats.FarmingAttempts == 0 {
		t.mu.Unlock()
		return
	}

	// 전송할 데이터 준비 (복사본 생성 - 레이스 컨디션 방지)
	t.stats.SessionDuration = int(time.Since(t.sessionStart).Seconds())
	period := time.Now().Format("2006-01-02")

	payload := Payload{
		SchemaVersion: schemaVer,
		AppVersion:    t.appVersion,
		OSType:        runtime.GOOS,
		SessionID:     t.sessionID,
		Period:        period,
		Stats:         t.copyStats(), // 복사본 사용
	}

	// 서명 생성
	signature := generateSignature(t.sessionID, period)

	// 마지막 전송 시간 업데이트 (lock 내에서)
	t.lastSentTime = time.Now()

	// 통계 리셋 (비동기 전송 전에 리셋하여 중복 전송 방지)
	t.stats = Stats{}
	t.saveState()
	t.mu.Unlock()

	// 비동기 전송 (메인 스레드 블로킹 방지)
	go t.sendAsync(payload, signature)
}

// copyStats 통계 복사본 생성 (맵 deep copy)
func (t *Telemetry) copyStats() Stats {
	copied := t.stats

	// 맵 deep copy
	if t.stats.EnhanceByLevel != nil {
		copied.EnhanceByLevel = make(map[int]int)
		for k, v := range t.stats.EnhanceByLevel {
			copied.EnhanceByLevel[k] = v
		}
	}
	if t.stats.SwordBattleStats != nil {
		copied.SwordBattleStats = make(map[string]*SwordBattleStat)
		for k, v := range t.stats.SwordBattleStats {
			vc := *v
			copied.SwordBattleStats[k] = &vc
		}
	}
	if t.stats.SpecialFoundByName != nil {
		copied.SpecialFoundByName = make(map[string]int)
		for k, v := range t.stats.SpecialFoundByName {
			copied.SpecialFoundByName[k] = v
		}
	}
	if t.stats.ItemFarmingStats != nil {
		copied.ItemFarmingStats = make(map[string]*ItemFarmingStat)
		for k, v := range t.stats.ItemFarmingStats {
			vc := *v
			copied.ItemFarmingStats[k] = &vc
		}
	}
	if t.stats.UpsetStatsByDiff != nil {
		copied.UpsetStatsByDiff = make(map[int]*UpsetStat)
		for k, v := range t.stats.UpsetStatsByDiff {
			vc := *v
			copied.UpsetStatsByDiff[k] = &vc
		}
	}
	if t.stats.SwordSaleStats != nil {
		copied.SwordSaleStats = make(map[string]*SwordSaleStat)
		for k, v := range t.stats.SwordSaleStats {
			vc := *v
			copied.SwordSaleStats[k] = &vc
		}
	}
	if t.stats.SwordEnhanceStats != nil {
		copied.SwordEnhanceStats = make(map[string]*SwordEnhanceStat)
		for k, v := range t.stats.SwordEnhanceStats {
			vc := *v
			copied.SwordEnhanceStats[k] = &vc
		}
	}
	if t.stats.Session != nil {
		sc := *t.stats.Session
		copied.Session = &sc
	}

	return copied
}

// sendAsync 비동기 전송 (복사된 payload 사용)
func (t *Telemetry) sendAsync(payload Payload, signature string) {
	data, err := json.Marshal(payload)
	if err != nil {
		logger.Error("[텔레메트리] JSON 직렬화 실패: %v", err)
		return
	}

	client := &http.Client{Timeout: sendTimeout}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		logger.Error("[텔레메트리] 요청 생성 실패: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Signature", signature)

	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("[텔레메트리] 전송 실패: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		logger.Debug("[텔레메트리] 전송 성공")
	} else {
		logger.Debug("[텔레메트리] 서버 응답 오류: %d", resp.StatusCode)
	}
}

// generateSignature 서명 생성
func generateSignature(sessionID, period string) string {
	h := sha256.Sum256([]byte(sessionID + period + getAppSecret()))
	return hex.EncodeToString(h[:])[:16]
}

// send HTTP 전송 (에러 로깅 포함)
func (t *Telemetry) send(payload Payload, signature string) {
	data, err := json.Marshal(payload)
	if err != nil {
		logger.Error("[텔레메트리] JSON 직렬화 실패: %v", err)
		return
	}

	client := &http.Client{Timeout: sendTimeout}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		logger.Error("[텔레메트리] 요청 생성 실패: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Signature", signature)

	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("[텔레메트리] 전송 실패: %v", err)
		return
	}
	defer resp.Body.Close()

	// 성공 시 상태 업데이트
	if resp.StatusCode == http.StatusOK {
		t.mu.Lock()
		t.stats = Stats{} // 통계 리셋
		t.saveState()
		t.mu.Unlock()
		logger.Debug("[텔레메트리] 전송 성공")
	} else {
		logger.Debug("[텔레메트리] 서버 응답 오류: %d", resp.StatusCode)
	}
}

// Flush 종료 시 강제 전송
func (t *Telemetry) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled {
		return
	}

	// 전송할 데이터가 있으면 전송
	if t.stats.TotalCycles > 0 || t.stats.TotalSwordsFound > 0 ||
		t.stats.BattleCount > 0 || t.stats.SalesCount > 0 || t.stats.FarmingAttempts > 0 {
		t.stats.SessionDuration = int(time.Since(t.sessionStart).Seconds())
		period := time.Now().Format("2006-01-02")

		payload := Payload{
			SchemaVersion: schemaVer,
			AppVersion:    t.appVersion,
			OSType:        runtime.GOOS,
			SessionID:     t.sessionID,
			Period:        period,
			Stats:         t.stats,
		}

		signature := generateSignature(t.sessionID, period)

		// 동기 전송 (종료 전 확실히 전송)
		data, err := json.Marshal(payload)
		if err == nil {
			client := &http.Client{Timeout: sendTimeout}
			req, _ := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
			if req != nil {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-App-Signature", signature)
				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}
		}
	}

	t.saveState()
}

func (t *Telemetry) loadState() {
	t.mu.Lock()
	defer t.mu.Unlock()

	st := t.loadStateUnlocked()
	t.enabled = st.Enabled
	t.sessionID = st.SessionID
	t.stats = st.Stats
	t.lastSentTime = time.Unix(st.LastSentTime, 0)

	if t.sessionID == "" {
		t.sessionID = uuid.New().String()
	}
}

func (t *Telemetry) loadStateUnlocked() state {
	data, err := os.ReadFile(t.statePath)
	if err != nil {
		return state{Enabled: true} // 기본값: 활성화
	}

	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return state{Enabled: true}
	}
	return st
}

func (t *Telemetry) saveState() {
	st := state{
		Enabled:      t.enabled,
		SessionID:    t.sessionID,
		LastSentTime: t.lastSentTime.Unix(),
		Stats:        t.stats,
		SessionStart: t.sessionStart.Unix(),
	}
	t.saveStateUnlocked(st)
}

func (t *Telemetry) saveStateUnlocked(st state) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(t.statePath, data, 0644)
}

func getStatePath() string {
	exe, err := os.Executable()
	if err != nil {
		return stateFile
	}
	return filepath.Join(filepath.Dir(exe), stateFile)
}
