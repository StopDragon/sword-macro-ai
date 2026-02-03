package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	endpoint     = "https://sword-ai.stopdragon.kr/api/telemetry"
	stateFile    = ".telemetry_state.json"
	schemaVer    = 1
	sendTimeout  = 5 * time.Second
	sendInterval = 5 * time.Minute
	appSecret    = "sw0rd-m4cr0-2026-s3cr3t-k3y"
)

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
	EnhanceAttempts int            `json:"enhance_attempts"`
	EnhanceSuccess  int            `json:"enhance_success"`
	EnhanceFail     int            `json:"enhance_fail"`
	EnhanceDestroy  int            `json:"enhance_destroy"`
	EnhanceByLevel  map[int]int    `json:"enhance_by_level,omitempty"` // level -> success count

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
	HiddenFound     int `json:"hidden_found"`
	TrashFound      int `json:"trash_found"`
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
func (t *Telemetry) RecordFarming(isHidden bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled {
		return
	}

	t.stats.FarmingAttempts++
	if isHidden {
		t.stats.HiddenFound++
	} else {
		t.stats.TrashFound++
	}
}

// TrySend 5분 간격으로 서버에 전송 시도
func (t *Telemetry) TrySend() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled {
		return
	}

	// 5분 경과 확인
	if time.Since(t.lastSentTime) < sendInterval {
		return
	}

	// 전송할 데이터가 없으면 스킵
	if t.stats.TotalCycles == 0 && t.stats.TotalSwordsFound == 0 &&
		t.stats.BattleCount == 0 && t.stats.SalesCount == 0 && t.stats.FarmingAttempts == 0 {
		return
	}

	// 전송할 데이터 준비
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

	// 서명 생성
	signature := generateSignature(t.sessionID, period)

	// 비동기 전송 (메인 스레드 블로킹 방지)
	go t.send(payload, signature)

	// 마지막 전송 시간 업데이트
	t.lastSentTime = time.Now()
}

// generateSignature 서명 생성
func generateSignature(sessionID, period string) string {
	h := sha256.Sum256([]byte(sessionID + period + appSecret))
	return hex.EncodeToString(h[:])[:16]
}

// send HTTP 전송
func (t *Telemetry) send(payload Payload, signature string) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: sendTimeout}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Signature", signature)

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// 성공 시 상태 업데이트
	if resp.StatusCode == http.StatusOK {
		t.mu.Lock()
		t.stats = Stats{} // 통계 리셋
		t.saveState()
		t.mu.Unlock()
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
