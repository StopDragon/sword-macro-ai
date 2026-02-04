package analysis

import (
	"fmt"
	"time"
)

// AlertType 알림 타입
type AlertType string

const (
	AlertInfo        AlertType = "info"
	AlertWarning     AlertType = "warning"
	AlertOpportunity AlertType = "opportunity"
	AlertDanger      AlertType = "danger"
)

// Alert 알림
type Alert struct {
	Type      AlertType `json:"type"`
	Icon      string    `json:"icon"`
	Message   string    `json:"message"`
	Priority  int       `json:"priority"` // 1-10
	Timestamp time.Time `json:"timestamp"`
	Expires   time.Time `json:"expires"` // 알림 만료 시간
}

// AlertEngine 알림 엔진
type AlertEngine struct {
	alerts       []Alert
	session      *SessionTracker
	risk         *RiskAnalysis
	strategy     *StrategyProfile
	maxAlerts    int
	lastAnalysis time.Time
}

// NewAlertEngine 새 알림 엔진 생성
func NewAlertEngine() *AlertEngine {
	return &AlertEngine{
		alerts:    make([]Alert, 0),
		maxAlerts: 5,
	}
}

// SetSession 세션 설정
func (e *AlertEngine) SetSession(session *SessionTracker) {
	e.session = session
}

// SetRisk 리스크 분석 설정
func (e *AlertEngine) SetRisk(risk *RiskAnalysis) {
	e.risk = risk
}

// SetStrategy 전략 설정
func (e *AlertEngine) SetStrategy(strategy *StrategyProfile) {
	e.strategy = strategy
}

// Update 알림 업데이트 (주기적 호출)
func (e *AlertEngine) Update() {
	// 1초 간격으로만 분석
	if time.Since(e.lastAnalysis) < time.Second {
		return
	}
	e.lastAnalysis = time.Now()

	// 기존 알림 중 만료된 것 제거
	e.removeExpiredAlerts()

	// 새 알림 생성
	newAlerts := e.checkAlerts()
	for _, alert := range newAlerts {
		e.addAlert(alert)
	}
}

// GetAlerts 현재 알림 반환
func (e *AlertEngine) GetAlerts() []Alert {
	return e.alerts
}

// GetTopAlerts 우선순위 높은 알림 N개 반환
func (e *AlertEngine) GetTopAlerts(n int) []Alert {
	if n > len(e.alerts) {
		n = len(e.alerts)
	}

	// 우선순위로 정렬 (이미 정렬되어 있다고 가정)
	return e.alerts[:n]
}

// checkAlerts 알림 생성 규칙
func (e *AlertEngine) checkAlerts() []Alert {
	var alerts []Alert

	// 리스크 기반 알림
	if e.risk != nil && e.strategy != nil {
		// 파산 위험 경고
		if e.risk.RuinProb > e.strategy.MaxRuinProb*100 {
			alerts = append(alerts, Alert{
				Type: AlertDanger,
				Icon: "🚨",
				Message: fmt.Sprintf("파산 위험 %.0f%% - 전략 기준(%.0f%%) 초과",
					e.risk.RuinProb, e.strategy.MaxRuinProb*100),
				Priority:  10,
				Timestamp: time.Now(),
				Expires:   time.Now().Add(30 * time.Second),
			})
		}

		// 목표 도달 알림
		if e.risk.CurrentLevel >= e.strategy.TargetLevel {
			alerts = append(alerts, Alert{
				Type:      AlertInfo,
				Icon:      "🎯",
				Message:   fmt.Sprintf("+%d 도달 - 전략 기준 판매 시점", e.risk.CurrentLevel),
				Priority:  9,
				Timestamp: time.Now(),
				Expires:   time.Now().Add(60 * time.Second),
			})
		}
	}

	// 세션 기반 알림
	if e.session != nil {
		report := e.session.GenerateReport()

		// 역배 승률 높음 알림
		if e.session.UpsetAttempts > 5 && report.UpsetWinRate > 40 {
			alerts = append(alerts, Alert{
				Type:      AlertOpportunity,
				Icon:      "⚡",
				Message:   fmt.Sprintf("역배 승률 %.0f%% - 평소(35%%)보다 높음!", report.UpsetWinRate),
				Priority:  7,
				Timestamp: time.Now(),
				Expires:   time.Now().Add(120 * time.Second),
			})
		}

		// 수익률 기반 알림
		if report.ROI > 50 {
			alerts = append(alerts, Alert{
				Type:      AlertInfo,
				Icon:      "📈",
				Message:   fmt.Sprintf("세션 수익률 +%.0f%% 달성", report.ROI),
				Priority:  6,
				Timestamp: time.Now(),
				Expires:   time.Now().Add(60 * time.Second),
			})
		}

		// 낙폭 경고
		if report.MaxDrawdown > 30 {
			alerts = append(alerts, Alert{
				Type:      AlertWarning,
				Icon:      "📉",
				Message:   fmt.Sprintf("낙폭 %.0f%% - 손실 관리 주의", report.MaxDrawdown),
				Priority:  8,
				Timestamp: time.Now(),
				Expires:   time.Now().Add(45 * time.Second),
			})
		}

		// 특수 아이템 발견 알림
		if e.session.SpecialFound > 0 {
			alerts = append(alerts, Alert{
				Type:      AlertOpportunity,
				Icon:      "✨",
				Message:   fmt.Sprintf("특수 아이템 %d개 발견!", e.session.SpecialFound),
				Priority:  5,
				Timestamp: time.Now(),
				Expires:   time.Now().Add(30 * time.Second),
			})
		}
	}

	return alerts
}

// addAlert 알림 추가
func (e *AlertEngine) addAlert(alert Alert) {
	// 중복 체크 (같은 메시지가 이미 있으면 스킵)
	for _, existing := range e.alerts {
		if existing.Message == alert.Message {
			return
		}
	}

	// 우선순위 순으로 삽입
	inserted := false
	for i, existing := range e.alerts {
		if alert.Priority > existing.Priority {
			// 해당 위치에 삽입
			e.alerts = append(e.alerts[:i], append([]Alert{alert}, e.alerts[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		e.alerts = append(e.alerts, alert)
	}

	// 최대 개수 초과 시 낮은 우선순위 제거
	if len(e.alerts) > e.maxAlerts {
		e.alerts = e.alerts[:e.maxAlerts]
	}
}

// removeExpiredAlerts 만료된 알림 제거
func (e *AlertEngine) removeExpiredAlerts() {
	now := time.Now()
	filtered := make([]Alert, 0)

	for _, alert := range e.alerts {
		if alert.Expires.After(now) {
			filtered = append(filtered, alert)
		}
	}

	e.alerts = filtered
}

// ClearAlerts 모든 알림 제거
func (e *AlertEngine) ClearAlerts() {
	e.alerts = make([]Alert, 0)
}

// FormatAlerts 알림 포맷팅 (오버레이용)
func FormatAlerts(alerts []Alert) string {
	if len(alerts) == 0 {
		return ""
	}

	result := "━━━━━━━━━━ 스마트 알림 ━━━━━━━━━━\n"
	for _, alert := range alerts {
		result += fmt.Sprintf("%s %s\n", alert.Icon, alert.Message)
	}

	return result
}

// FormatAlertsCompact 컴팩트 포맷 (한 줄)
func FormatAlertsCompact(alerts []Alert) string {
	if len(alerts) == 0 {
		return ""
	}

	result := ""
	for i, alert := range alerts {
		if i > 0 {
			result += " | "
		}
		result += alert.Icon + " " + alert.Message
	}

	return result
}
