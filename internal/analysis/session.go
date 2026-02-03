package analysis

import (
	"fmt"
	"math"
	"time"
)

// SessionTracker 세션 추적기
type SessionTracker struct {
	SessionID  string
	StartTime  time.Time
	GoldHistory []GoldSnapshot

	// 활동 카운터
	EnhanceCount int
	BattleCount  int
	BattleWins   int
	SalesCount   int
	HiddenFound  int
	UpsetAttempts int
	UpsetWins    int

	// 골드 추적
	StartingGold int
	CurrentGold  int
	PeakGold     int
	LowestGold   int
}

// GoldSnapshot 골드 스냅샷 (ROI, 드로다운 계산용)
type GoldSnapshot struct {
	Timestamp time.Time
	Gold      int
}

// SessionReport 세션 리포트
type SessionReport struct {
	SessionID string        `json:"session_id"`
	Duration  time.Duration `json:"duration"`

	// 자본 변화
	StartingGold int `json:"starting_gold"`
	EndingGold   int `json:"ending_gold"`
	PeakGold     int `json:"peak_gold"`
	LowestGold   int `json:"lowest_gold"`

	// 성과 지표
	ROI         float64 `json:"roi"`          // 수익률 (%)
	MaxDrawdown float64 `json:"max_drawdown"` // 최대 낙폭 (%)
	SharpeRatio float64 `json:"sharpe_ratio"` // 위험 대비 수익

	// 활동 요약
	EnhanceCount int `json:"enhance_count"`
	BattleCount  int `json:"battle_count"`
	SalesCount   int `json:"sales_count"`
	HiddenFound  int `json:"hidden_found"`

	// 전략 분석
	WinRate      float64 `json:"win_rate"`       // 배틀 승률
	UpsetWinRate float64 `json:"upset_win_rate"` // 역배 승률
}

// NewSessionTracker 새 세션 추적기 생성
func NewSessionTracker(startingGold int) *SessionTracker {
	return &SessionTracker{
		SessionID:    generateSessionID(),
		StartTime:    time.Now(),
		StartingGold: startingGold,
		CurrentGold:  startingGold,
		PeakGold:     startingGold,
		LowestGold:   startingGold,
		GoldHistory:  []GoldSnapshot{{Timestamp: time.Now(), Gold: startingGold}},
	}
}

func generateSessionID() string {
	return time.Now().Format("20060102_150405")
}

// RecordGold 골드 변화 기록
func (s *SessionTracker) RecordGold(gold int) {
	s.CurrentGold = gold
	s.GoldHistory = append(s.GoldHistory, GoldSnapshot{
		Timestamp: time.Now(),
		Gold:      gold,
	})

	if gold > s.PeakGold {
		s.PeakGold = gold
	}
	if gold < s.LowestGold {
		s.LowestGold = gold
	}
}

// RecordEnhance 강화 기록
func (s *SessionTracker) RecordEnhance() {
	s.EnhanceCount++
}

// RecordBattle 배틀 기록
func (s *SessionTracker) RecordBattle(won bool, isUpset bool) {
	s.BattleCount++
	if won {
		s.BattleWins++
	}
	if isUpset {
		s.UpsetAttempts++
		if won {
			s.UpsetWins++
		}
	}
}

// RecordSale 판매 기록
func (s *SessionTracker) RecordSale() {
	s.SalesCount++
}

// RecordHidden 히든 발견 기록
func (s *SessionTracker) RecordHidden() {
	s.HiddenFound++
}

// GenerateReport 세션 리포트 생성
func (s *SessionTracker) GenerateReport() *SessionReport {
	duration := time.Since(s.StartTime)

	// ROI 계산
	roi := 0.0
	if s.StartingGold > 0 {
		roi = float64(s.CurrentGold-s.StartingGold) / float64(s.StartingGold) * 100
	}

	// 최대 낙폭 계산
	maxDrawdown := s.calculateMaxDrawdown()

	// 샤프 비율 계산 (간이 버전)
	sharpeRatio := s.calculateSharpeRatio()

	// 승률 계산
	winRate := 0.0
	if s.BattleCount > 0 {
		winRate = float64(s.BattleWins) / float64(s.BattleCount) * 100
	}

	upsetWinRate := 0.0
	if s.UpsetAttempts > 0 {
		upsetWinRate = float64(s.UpsetWins) / float64(s.UpsetAttempts) * 100
	}

	return &SessionReport{
		SessionID:    s.SessionID,
		Duration:     duration,
		StartingGold: s.StartingGold,
		EndingGold:   s.CurrentGold,
		PeakGold:     s.PeakGold,
		LowestGold:   s.LowestGold,
		ROI:          roi,
		MaxDrawdown:  maxDrawdown,
		SharpeRatio:  sharpeRatio,
		EnhanceCount: s.EnhanceCount,
		BattleCount:  s.BattleCount,
		SalesCount:   s.SalesCount,
		HiddenFound:  s.HiddenFound,
		WinRate:      winRate,
		UpsetWinRate: upsetWinRate,
	}
}

// calculateMaxDrawdown 최대 낙폭 계산
func (s *SessionTracker) calculateMaxDrawdown() float64 {
	if len(s.GoldHistory) < 2 {
		return 0
	}

	maxDrawdown := 0.0
	peak := s.GoldHistory[0].Gold

	for _, snapshot := range s.GoldHistory {
		if snapshot.Gold > peak {
			peak = snapshot.Gold
		}
		if peak > 0 {
			drawdown := float64(peak-snapshot.Gold) / float64(peak) * 100
			if drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}

	return maxDrawdown
}

// calculateSharpeRatio 샤프 비율 계산 (간이 버전)
// Sharpe Ratio = (평균 수익률 - 무위험 수익률) / 표준편차
func (s *SessionTracker) calculateSharpeRatio() float64 {
	if len(s.GoldHistory) < 3 {
		return 0
	}

	// 수익률 배열 계산
	var returns []float64
	for i := 1; i < len(s.GoldHistory); i++ {
		prev := s.GoldHistory[i-1].Gold
		curr := s.GoldHistory[i].Gold
		if prev > 0 {
			ret := float64(curr-prev) / float64(prev)
			returns = append(returns, ret)
		}
	}

	if len(returns) < 2 {
		return 0
	}

	// 평균 수익률
	var sum float64
	for _, r := range returns {
		sum += r
	}
	meanReturn := sum / float64(len(returns))

	// 표준편차
	var varianceSum float64
	for _, r := range returns {
		varianceSum += (r - meanReturn) * (r - meanReturn)
	}
	stdDev := math.Sqrt(varianceSum / float64(len(returns)))

	if stdDev == 0 {
		return 0
	}

	// 무위험 수익률은 0으로 가정 (게임이므로)
	return meanReturn / stdDev
}

// FormatDuration 시간 포맷팅
func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%d시간 %02d분 %02d초", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%d분 %02d초", m, s)
	}
	return fmt.Sprintf("%d초", s)
}
