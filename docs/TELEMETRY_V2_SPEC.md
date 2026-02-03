# 텔레메트리 v2 기획안

> 검 종류별 통계 수집 및 커뮤니티 데이터 기반 분석 기능

## 목표

사용자들의 게임 데이터를 수집하여 **실제 데이터 기반** 분석 정보 제공:
- 검 종류별 승률/역배 성공률
- 특수 아이템 출현 확률
- 레벨별 실제 강화 성공률 (이론값 vs 실측값)
- 내 검의 커뮤니티 평균 대비 성적

---

## 1. 새로 수집할 데이터

### 1.1 검 종류별 배틀 통계
| 필드 | 타입 | 설명 |
|------|------|------|
| `sword_name` | string | 검 이름 (예: "불꽃검", "얼음검") |
| `level` | int | 검 레벨 |
| `battle_count` | int | 총 배틀 횟수 |
| `battle_wins` | int | 승리 횟수 |
| `upset_attempts` | int | 역배 시도 횟수 |
| `upset_wins` | int | 역배 성공 횟수 |

### 1.2 특수 아이템 발견 통계
| 필드 | 타입 | 설명 |
|------|------|------|
| `special_name` | string | 특수 아이템 이름 |
| `count` | int | 발견 횟수 |

### 1.3 레벨별 역배 통계
| 필드 | 타입 | 설명 |
|------|------|------|
| `level_diff` | int | 레벨 차이 (1, 2, 3) |
| `attempts` | int | 시도 횟수 |
| `wins` | int | 성공 횟수 |
| `gold_earned` | int | 총 획득 골드 |

### 1.4 검 종류별 판매 통계
| 필드 | 타입 | 설명 |
|------|------|------|
| `sword_name` | string | 검 이름 |
| `level` | int | 판매 시 레벨 |
| `price` | int | 판매 가격 |
| `count` | int | 판매 횟수 |

---

## 2. 데이터 구조 변경

### 2.1 클라이언트 (internal/telemetry/telemetry.go)

```go
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

// Stats 확장
type Stats struct {
    // === 기존 필드 유지 ===
    TotalCycles      int         `json:"total_cycles"`
    SuccessfulCycles int         `json:"successful_cycles"`
    // ... (기존 필드들)

    // === 새로 추가 ===

    // 검 종류별 배틀 통계: "불꽃검" -> SwordBattleStat
    SwordBattleStats map[string]*SwordBattleStat `json:"sword_battle_stats,omitempty"`

    // 특수 아이템 발견 통계: "용검" -> 3
    SpecialFoundByName map[string]int `json:"special_found_by_name,omitempty"`

    // 레벨차별 역배 통계: 1 -> UpsetStat, 2 -> UpsetStat, 3 -> UpsetStat
    UpsetStatsByDiff map[int]*UpsetStat `json:"upset_stats_by_diff,omitempty"`

    // 검+레벨별 판매 통계: "불꽃검_10" -> SwordSaleStat
    SwordSaleStats map[string]*SwordSaleStat `json:"sword_sale_stats,omitempty"`
}
```

### 2.2 서버 (cmd/sword-api/main.go)

```go
// StatsStore 확장
type StatsStore struct {
    // ... 기존 필드 ...

    // 검 종류별 통계 (전체 커뮤니티)
    swordBattleStats  map[string]*SwordBattleStat
    specialFoundByName map[string]int
    upsetStatsByDiff  map[int]*UpsetStat
    swordSaleStats    map[string]*SwordSaleStat
}
```

---

## 3. 새로운 API 엔드포인트

### 3.1 검 종류별 승률 랭킹
```
GET /api/stats/swords
```

**Response:**
```json
{
  "swords": [
    {
      "name": "불꽃검",
      "battle_count": 15420,
      "win_rate": 65.2,
      "upset_win_rate": 38.5
    },
    {
      "name": "얼음검",
      "battle_count": 12350,
      "win_rate": 62.1,
      "upset_win_rate": 35.2
    }
  ]
}
```

### 3.2 특수 아이템 출현 확률
```
GET /api/stats/special
```

**Response:**
```json
{
  "total_farming": 1250000,
  "special": [
    {"name": "용검", "count": 125, "rate": 0.01},
    {"name": "천사검", "count": 312, "rate": 0.025},
    {"name": "악마검", "count": 287, "rate": 0.023}
  ]
}
```

### 3.3 역배 실측 승률
```
GET /api/stats/upset
```

**Response:**
```json
{
  "by_level_diff": {
    "1": {"attempts": 50000, "wins": 17500, "win_rate": 35.0, "theory": 35.0},
    "2": {"attempts": 25000, "wins": 5000, "win_rate": 20.0, "theory": 20.0},
    "3": {"attempts": 10000, "wins": 1050, "win_rate": 10.5, "theory": 10.0}
  }
}
```

### 3.4 내 검 분석 (검 이름으로 조회)
```
GET /api/stats/sword/{name}
```

**Response:**
```json
{
  "name": "불꽃검",
  "community_stats": {
    "battle_count": 15420,
    "win_rate": 65.2,
    "upset_win_rate": 38.5,
    "avg_sale_price_by_level": {
      "10": 125000,
      "11": 320000,
      "12": 920000
    }
  },
  "ranking": 3
}
```

---

## 4. 클라이언트 변경사항

### 4.1 새로운 Record 함수 추가

```go
// RecordBattleWithSword 검 종류 포함 배틀 기록
func (t *Telemetry) RecordBattleWithSword(
    mySwordName string,
    myLevel int,
    oppLevel int,
    won bool,
    goldEarned int,
)

// RecordSpecialWithName 특수 아이템 이름 포함 기록
func (t *Telemetry) RecordSpecialWithName(swordName string)

// RecordSaleWithSword 검 종류 포함 판매 기록
func (t *Telemetry) RecordSaleWithSword(swordName string, level int, price int)
```

### 4.2 engine.go 수정 필요 위치

| 함수 | 수정 내용 |
|------|----------|
| `runBattleMode()` | 배틀 시 검 이름 추출 후 `RecordBattleWithSword()` 호출 |
| `runSpecialMode()` | 특수 아이템 발견 시 이름 추출 후 `RecordSpecialWithName()` 호출 |
| `runGoldMineMode()` | 판매 시 검 이름 추출 후 `RecordSaleWithSword()` 호출 |

### 4.3 parser.go 수정 필요

특수 아이템 이름 추출을 위한 패턴 추가:
```go
// 특수 아이템 이름 패턴
specialNamePattern = regexp.MustCompile(`(?:히든|hidden|특수|special).*?『([^』]+)』`)

// ExtractSpecialName 특수 아이템 이름 추출
func ExtractSpecialName(text string) string
```

---

## 5. 내 프로필 분석 개선

### 5.1 현재 (v1)
- 이론 기반 강화 확률
- 이론 기반 역배 기대값

### 5.2 개선 후 (v2)
```
=== 내 프로필 분석 ===

⚔️ 내 검 정보
   보유 검: [+10] 불꽃검

📊 불꽃검 커뮤니티 통계
   전체 승률: 65.2% (3위)
   역배 승률: 38.5% (이론: 35%)
   평균 판매가: 12.5만G (평균 대비 +5%)

⚡ 역배 분석 (실측 데이터)
   레벨차 | 이론 | 실측  | 내 검
   +1     | 35%  | 35.2% | 38.5% ▲
   +2     | 20%  | 20.1% | 22.3% ▲
   +3     | 10%  | 10.5% | 11.2% ▲

💡 불꽃검은 역배에 강한 검입니다!
```

---

## 6. 프라이버시 고려사항

### 수집하는 정보
- 검 이름 (게임 내 아이템명)
- 레벨, 승패, 가격 등 통계

### 수집하지 않는 정보
- 사용자 닉네임 (@username)
- 상대방 정보
- 개인 식별 가능 정보

### README 업데이트 필요
```markdown
### 수집 항목
- 강화 시도/성공/실패/파괴 횟수
- 배틀 횟수 및 승패
- 파밍/판매 통계
- 앱 버전, OS 종류
- **검 종류별 통계 (이름, 승률)** ← 추가
- **특수 아이템 발견 종류** ← 추가
```

---

## 7. 구현 순서

### Phase 1: 데이터 구조 (클라이언트)
1. [ ] `telemetry.go` - 새 Stats 필드 추가
2. [ ] `telemetry.go` - 새 Record 함수 추가
3. [ ] `parser.go` - 특수 아이템 이름 추출 패턴 추가

### Phase 2: 데이터 수집 (클라이언트)
4. [ ] `engine.go` - `runBattleMode()` 수정
5. [ ] `engine.go` - `runSpecialMode()` 수정
6. [ ] `engine.go` - `runGoldMineMode()` 수정

### Phase 3: 서버 API
7. [ ] `main.go` - StatsStore 확장
8. [ ] `main.go` - 텔레메트리 수신 로직 수정
9. [ ] `main.go` - 새 API 엔드포인트 추가

### Phase 4: 데이터 조회 (클라이언트)
10. [ ] `data.go` - 새 API 조회 함수 추가
11. [ ] `engine.go` - `showMyProfile()` 개선

### Phase 5: 마무리
12. [ ] README.md 업데이트
13. [ ] 서버 배포
14. [ ] v1.1.0 릴리스

---

## 8. 스키마 버전

현재: `schema_version: 1`
변경: `schema_version: 2`

서버는 하위 호환성 유지:
- v1 클라이언트: 기존 필드만 처리
- v2 클라이언트: 새 필드 포함 처리

---

## 9. 예상 효과

| 지표 | 현재 | 목표 |
|------|------|------|
| 프로필 분석 정확도 | 이론값만 | 실측 데이터 기반 |
| 제공 정보 | 5개 | 10개+ |
| 사용자 가치 | 계산기 수준 | 커뮤니티 인사이트 |

---

## 10. 실시간 분석 시스템

### 10.1 리스크 계산기

현재 상태에서 목표 달성 확률과 파산 위험을 실시간 계산:

```go
// RiskAnalysis 리스크 분석 결과
type RiskAnalysis struct {
    CurrentLevel    int     `json:"current_level"`
    CurrentGold     int     `json:"current_gold"`
    TargetLevel     int     `json:"target_level"`

    // 확률 분석
    SuccessProb     float64 `json:"success_prob"`      // 목표 도달 확률
    RuinProb        float64 `json:"ruin_prob"`         // 파산 확률
    ExpectedGold    int     `json:"expected_gold"`     // 기대 최종 골드
    ExpectedTrials  int     `json:"expected_trials"`   // 예상 시도 횟수

    // 켈리 기준
    KellyBetRatio   float64 `json:"kelly_bet_ratio"`   // 최적 배팅 비율
    MaxDrawdown     float64 `json:"max_drawdown"`      // 예상 최대 낙폭

    // 추천
    Recommendation  string  `json:"recommendation"`    // "enhance", "sell", "wait"
    Warning         string  `json:"warning,omitempty"` // 경고 메시지
}

// CalcRisk 리스크 계산
func CalcRisk(currentLevel, currentGold, targetLevel int) *RiskAnalysis
```

**출력 예시:**
```
⚠️ 리스크 분석 (현재: +8, 50만G)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
목표 +10 도달: 35% 확률
파산 위험: 22%
예상 소요: 15만G (평균 12회)

📊 켈리 기준 배팅: 골드의 8%
📉 예상 최대 낙폭: -35%

💡 추천: 강화 진행 (리스크 허용 범위)
⚠️ 경고: +12 목표는 파산 확률 68%
```

### 10.2 세션 분석

세션별 성과를 추적하고 리포트 생성:

```go
// SessionStats 세션 통계
type SessionStats struct {
    SessionID     string        `json:"session_id"`
    StartTime     time.Time     `json:"start_time"`
    Duration      time.Duration `json:"duration"`

    // 자본 변화
    StartingGold  int     `json:"starting_gold"`
    EndingGold    int     `json:"ending_gold"`
    PeakGold      int     `json:"peak_gold"`      // 최고점
    LowestGold    int     `json:"lowest_gold"`    // 최저점

    // 성과 지표
    ROI           float64 `json:"roi"`            // 수익률 (%)
    MaxDrawdown   float64 `json:"max_drawdown"`   // 최대 낙폭 (%)
    SharpeRatio   float64 `json:"sharpe_ratio"`   // 위험 대비 수익

    // 활동 요약
    EnhanceCount  int     `json:"enhance_count"`
    BattleCount   int     `json:"battle_count"`
    SalesCount    int     `json:"sales_count"`
    SpecialFound  int     `json:"special_found"`

    // 전략 분석
    AvgSellLevel  float64 `json:"avg_sell_level"` // 평균 판매 레벨
    WinRate       float64 `json:"win_rate"`       // 배틀 승률
    UpsetWinRate  float64 `json:"upset_win_rate"` // 역배 승률
}
```

**세션 리포트 출력:**
```
📊 세션 리포트
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⏱️ 플레이 시간: 2시간 15분

💰 자본 변화
   시작: 50만G → 종료: 85만G
   최고: 120만G | 최저: 35만G

📈 성과
   수익률: +70%
   최대 낙폭: -30%
   샤프 비율: 1.8 (양호)

🎮 활동
   강화: 156회 (성공률 42%)
   배틀: 23회 (승률 65%)
   판매: 12회 (평균 +9.2강)

💡 분석
   오늘 역배 승률이 평소보다 8% 높았습니다
   +10 이상에서 판매하면 수익률 +15% 예상
```

### 10.3 전략 프로필

저장 가능한 전략 설정:

```go
// StrategyProfile 전략 프로필
type StrategyProfile struct {
    Name          string    `json:"name"`
    Description   string    `json:"description"`

    // 강화 전략
    TargetLevel   int       `json:"target_level"`    // 목표 레벨
    SellLevels    []int     `json:"sell_levels"`     // 판매 기준 레벨들
    StopLossGold  int       `json:"stop_loss_gold"`  // 손절 기준 골드

    // 배틀 전략
    EnableBattle  bool      `json:"enable_battle"`   // 배틀 활성화
    MaxUpsetDiff  int       `json:"max_upset_diff"`  // 최대 역배 레벨차
    MinBattleGold int       `json:"min_battle_gold"` // 배틀 최소 골드

    // 리스크 관리
    MaxBetRatio   float64   `json:"max_bet_ratio"`   // 최대 배팅 비율
    MaxRuinProb   float64   `json:"max_ruin_prob"`   // 허용 파산 확률

    // 자동화
    AutoSell      bool      `json:"auto_sell"`       // 자동 판매
    AutoBattle    bool      `json:"auto_battle"`     // 자동 배틀

    CreatedAt     time.Time `json:"created_at"`
    LastUsed      time.Time `json:"last_used"`
}

// 기본 제공 전략들
var DefaultStrategies = []StrategyProfile{
    {
        Name:         "안전한 10강러",
        Description:  "저위험 안정적 수익",
        TargetLevel:  10,
        SellLevels:   []int{10},
        MaxUpsetDiff: 1,
        MaxBetRatio:  0.05,
        MaxRuinProb:  0.15,
    },
    {
        Name:         "공격적 12강러",
        Description:  "고위험 고수익",
        TargetLevel:  12,
        SellLevels:   []int{12, 11},
        MaxUpsetDiff: 2,
        MaxBetRatio:  0.15,
        MaxRuinProb:  0.35,
    },
    {
        Name:         "역배 전문가",
        Description:  "배틀 중심 플레이",
        TargetLevel:  8,
        SellLevels:   []int{8, 9, 10},
        MaxUpsetDiff: 3,
        MaxBetRatio:  0.10,
        EnableBattle: true,
        AutoBattle:   true,
    },
    {
        Name:         "특수 헌터",
        Description:  "특수 아이템 파밍 전문",
        TargetLevel:  5,
        SellLevels:   []int{5, 6, 7},
        MaxUpsetDiff: 0,
        EnableBattle: false,
    },
}
```

**전략 선택 UI:**
```
=== 전략 프로필 ===
1. 안전한 10강러 (저위험) ◀ 현재
2. 공격적 12강러 (고위험)
3. 역배 전문가 (배틀 중심)
4. 특수 헌터 (파밍 전문)
5. [커스텀] 내 전략
0. 뒤로

현재 전략: 안전한 10강러
- 목표: +10 달성 후 판매
- 파산 허용: 15% 이하
- 역배: +1까지만
```

### 10.4 스마트 알림 (오버레이)

실시간 로그 아래에 표시되는 인사이트:

```go
// Alert 알림 타입
type Alert struct {
    Type      string    `json:"type"`      // "info", "warning", "opportunity", "danger"
    Icon      string    `json:"icon"`      // 이모지
    Message   string    `json:"message"`
    Priority  int       `json:"priority"`  // 1-10
    Timestamp time.Time `json:"timestamp"`
    Expires   time.Time `json:"expires"`   // 알림 만료 시간
}

// AlertEngine 알림 엔진
type AlertEngine struct {
    alerts       []Alert
    sessionStats *SessionStats
    riskAnalysis *RiskAnalysis
    strategy     *StrategyProfile
}

// 알림 생성 규칙
func (e *AlertEngine) CheckAlerts() []Alert {
    var alerts []Alert

    // 리스크 경고
    if e.riskAnalysis.RuinProb > e.strategy.MaxRuinProb {
        alerts = append(alerts, Alert{
            Type:    "danger",
            Icon:    "🚨",
            Message: fmt.Sprintf("파산 위험 %.0f%% - 전략 기준(%.0f%%) 초과",
                e.riskAnalysis.RuinProb*100, e.strategy.MaxRuinProb*100),
            Priority: 10,
        })
    }

    // 기회 알림
    if e.sessionStats.UpsetWinRate > 0.40 { // 역배 승률 40% 이상
        alerts = append(alerts, Alert{
            Type:    "opportunity",
            Icon:    "⚡",
            Message: fmt.Sprintf("역배 승률 %.0f%% - 평소보다 높음!",
                e.sessionStats.UpsetWinRate*100),
            Priority: 7,
        })
    }

    // 판매 추천
    if e.sessionStats.ROI > 0.5 && e.riskAnalysis.RuinProb > 0.3 {
        alerts = append(alerts, Alert{
            Type:    "warning",
            Icon:    "💡",
            Message: "수익률 50%+ 달성 - 일부 익절 고려",
            Priority: 6,
        })
    }

    return alerts
}
```

**오버레이 출력 예시:**
```
━━━━━━━━━━━━━ 실행 로그 ━━━━━━━━━━━━━
[14:23:15] 강화 성공! +8 → +9
[14:23:18] 강화 유지 +9
[14:23:21] 강화 성공! +9 → +10
[14:23:25] 판매 완료: 12만G

━━━━━━━━━━ 스마트 알림 ━━━━━━━━━━
⚡ 역배 승률 42% - 평소(35%)보다 높음!
💡 +10 도달 - 전략 기준 판매 시점
📊 세션 수익률 +45% (1시간)
⚠️ 파산 위험 18% - 주의 필요
```

### 10.5 통합 아키텍처

```
┌─────────────────────────────────────────┐
│              Game Engine                │
├─────────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │ Session │  │  Risk   │  │Strategy │ │
│  │ Tracker │  │ Calc    │  │ Engine  │ │
│  └────┬────┘  └────┬────┘  └────┬────┘ │
│       │            │            │       │
│       └────────────┼────────────┘       │
│                    │                    │
│            ┌───────▼───────┐            │
│            │ Alert Engine  │            │
│            └───────┬───────┘            │
│                    │                    │
│            ┌───────▼───────┐            │
│            │   Overlay UI  │            │
│            │ (로그 + 알림) │            │
│            └───────────────┘            │
└─────────────────────────────────────────┘
                    │
                    ▼
         ┌─────────────────┐
         │   Telemetry     │
         │   (서버 전송)   │
         └─────────────────┘
```

---

## 11. 추가 연구 과제 (RL 학습 환경 참고)

> 참고: [검키우기 매크로, RL 학습 환경 구현](https://velog.io/@ulurulu) - 강화학습 기반 최적 전략 연구

### 11.1 확률표 검증
현재 서버에 하드코딩된 확률표가 실제와 맞는지 검증 필요:

```
Level | 서버설정 | 실측(추정)
------+----------+-----------
+9    | 30%      | ???
+10   | 25%      | ???
+11   | 20%      | ???
```

**수집 방법:**
- 텔레메트리에서 레벨별 강화 결과 집계
- `성공률 = EnhanceByLevel[level] / EnhanceAttemptsByLevel[level]`

### 11.2 골드 의존성 검증 (중요!)
RL 연구에서 발견된 가설: **보유 골드에 따라 강화 확률이 달라질 수 있음**

> "600만원 이후로 안가는거 보면 budget에 따라 확률이 조금 바뀌는 것이 아닌가"

**새로 수집할 데이터:**
```go
// 골드 구간별 강화 통계
type EnhanceByGoldRange struct {
    GoldRange     string `json:"gold_range"` // "0-100K", "100K-1M", "1M-10M", "10M+"
    Level         int    `json:"level"`
    Attempts      int    `json:"attempts"`
    Success       int    `json:"success"`
    Hold          int    `json:"hold"`
    Destroy       int    `json:"destroy"`
}
```

**가설 검증:**
- 골드 구간별 성공률 비교
- 통계적 유의성 검정 (chi-square test)

### 11.3 최적 전략 도출
수집된 데이터로 **최적 판매 시점** 계산:

```
기대 수익 = Σ (레벨별 판매가 × 도달 확률) - Σ (강화 비용)
```

**제공할 분석:**
- 현재 레벨에서 계속 강화 vs 판매 기대값 비교
- "몇 강에서 팔아야 하나요?" 에 대한 데이터 기반 답변
- 골드별 최적 전략 (저자본 vs 고자본)

### 11.4 새 API 엔드포인트

```
GET /api/stats/probability-verification
```

**Response:**
```json
{
  "sample_size": 125000,
  "by_level": {
    "9": {
      "official": {"success": 30, "hold": 45, "destroy": 25},
      "measured": {"success": 29.8, "hold": 45.3, "destroy": 24.9},
      "confidence": 0.95
    }
  },
  "budget_dependency": {
    "hypothesis": "Budget may affect probability",
    "evidence_strength": "moderate",
    "details": {
      "low_budget": {"success_rate": 30.2},
      "high_budget": {"success_rate": 28.1}
    }
  }
}
```

```
GET /api/strategy/optimal-sell-point?level=8&gold=500000
```

**Response:**
```json
{
  "current_level": 8,
  "current_gold": 500000,
  "recommendation": {
    "action": "enhance",
    "target_sell_level": 10,
    "expected_profit": 125000,
    "risk_of_ruin": 0.15
  },
  "alternatives": [
    {"sell_at": 9, "expected_profit": 85000, "risk": 0.08},
    {"sell_at": 10, "expected_profit": 125000, "risk": 0.15},
    {"sell_at": 11, "expected_profit": 180000, "risk": 0.35}
  ]
}
```

### 11.5 프로필 분석 개선 (v3)

```
=== 내 프로필 분석 ===

⚔️ 내 검 정보
   보유 검: [+8] 불꽃검
   보유 골드: 50만G

📊 확률표 검증 (커뮤니티 12.5만 샘플)
   +8→+9: 공식 40% | 실측 39.8% ✓
   +9→+10: 공식 30% | 실측 29.2% ⚠️ (골드 의존성 의심)

🎯 최적 전략 (50만G 기준)
   추천: +10까지 강화 후 판매
   기대 수익: +12.5만G
   파산 확률: 15%

⚡ 대안 비교
   +9 판매: 기대 +8.5만G, 위험 8%
   +10 판매: 기대 +12.5만G, 위험 15% ◀ 추천
   +11 판매: 기대 +18만G, 위험 35%

💡 고자본(1000만G+)에서는 확률이 낮아질 수 있습니다 (검증 중)
```

---

## 12. 구현 우선순위 재정리

### Phase 1: 기본 데이터 수집 (v1.1)
- [ ] 검 종류별 배틀 통계
- [ ] 특수 아이템 발견 종류
- [ ] 레벨별 역배 실측
- [ ] **세션 통계 기본 (시작/종료 골드, 활동 횟수)**

### Phase 2: 실시간 분석 (v1.2)
- [ ] **리스크 계산기 (파산 확률, 켈리 기준)**
- [ ] **세션 분석 (ROI, 드로다운)**
- [ ] 레벨별 강화 결과 상세 수집 (성공/유지/파괴)
- [ ] 골드 구간별 강화 결과 수집

### Phase 3: 전략 시스템 (v1.3)
- [ ] **전략 프로필 (기본 4종 + 커스텀)**
- [ ] **스마트 알림 오버레이**
- [ ] 확률표 검증 API

### Phase 4: 고급 분석 (v1.4)
- [ ] 최적 판매 시점 계산 엔진
- [ ] 전략 추천 API
- [ ] 프로필 분석 v3
- [ ] 시간대별 패턴 분석
- [ ] 스트릭/천장 시스템 검증

---

*작성일: 2026-02-03*
*버전: Draft 1.2*
*참고: RL 학습 환경 구현 (velog.io/@ulurulu)*
