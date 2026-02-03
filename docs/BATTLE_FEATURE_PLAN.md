# 자동 배틀 및 데이터 수집 확장 기획안

## 1. 개요

### 1.1 목표
- **자동 역배 시스템**: 내 검보다 1~3레벨 높은 상대와 자동 대결하여 고배당 승리 노림
- **포괄적 데이터 수집**: 게임 내 모든 행동을 기록하여 최적 전략 도출

### 1.2 게임 메커니즘 이해

```
/프로필     → 내 상태 확인 (레벨, 골드, 전적)
/랭킹       → 서버 내 유저 목록 + 레벨 확인
/배틀 @유저 → 특정 유저와 대결
/판매       → 현재 검 판매 (골드 획득 + 새 검 획득)
/강화       → 검 레벨업 시도 (0강은 파괴)
```

---

## 2. 자동 배틀 모드 설계

### 2.1 역배 전략 (Underdog Betting)

```
내 검: +10
┌─────────────────────────────────────────┐
│  상대 레벨  │  예상 승률  │  보상 배율  │
├─────────────────────────────────────────┤
│    +10      │    50%     │    1.0x    │
│    +11      │    40%     │    1.5x    │  ← 적정 타겟
│    +12      │    30%     │    2.0x    │  ← 적정 타겟
│    +13      │    20%     │    3.0x    │  ← 고위험 고수익
│    +14+     │    10%     │    5.0x+   │  ← 너무 위험
└─────────────────────────────────────────┘
```

### 2.2 자동 배틀 플로우

```
┌──────────────────────────────────────────────────────────┐
│                    ModeBattle 루프                        │
├──────────────────────────────────────────────────────────┤
│  1. /프로필 → 내 레벨 파싱 (예: +10)                      │
│  2. /랭킹 → 서버 유저 목록 파싱                           │
│  3. 타겟 선정: 내 레벨 +1~3 범위의 유저 필터링             │
│  4. /배틀 @타겟유저 → 대결 실행                           │
│  5. 결과 파싱 → 승/패, 획득 골드                          │
│  6. 텔레메트리 기록                                       │
│  7. 쿨다운 대기 (rate limit 방지)                         │
│  8. 반복                                                  │
└──────────────────────────────────────────────────────────┘
```

### 2.3 OCR 파싱 패턴

```go
// 프로필 파싱
type Profile struct {
    Name       string  // @정진하
    Level      int     // +16
    SwordName  string  // 모래의 우주적 종언의 섭리
    Wins       int     // 34승
    Losses     int     // 22패
    Gold       int     // 18 G
}

// 랭킹 파싱 (배틀 랭킹에서 레벨 추출)
type RankingEntry struct {
    Rank     int     // 1위
    Username string  // @검키지지
    Level    int     // +20 (강화 랭킹에서)
    Wins     int     // 2255승
    Losses   int     // 838패
}

// 배틀 결과 파싱
type BattleResult struct {
    Winner      string  // @설민재
    Loser       string  // @정지용
    WinnerLevel int     // +14
    LoserLevel  int     // +16
    GoldEarned  int     // 2,407,543G
    IsUpset     bool    // 역배 여부 (낮은 레벨이 이김)
}
```

### 2.4 메뉴 UI 추가

```
=== 카카오톡 검키우기 ===
1. 강화 목표 달성
2. 히든 검 뽑기
3. 골드 채굴 (돈벌기)
4. 자동 배틀 (역배)     ← NEW
5. 옵션 설정
0. 종료

선택: 4

=== 자동 배틀 설정 ===
역배 레벨 차이 (1-3): 2
배틀 간격 (초): 5
목표 승수 (0=무제한): 10

⏱️ 30분간 자동 배틀을 시작합니다...

📊 내 프로필: +12 불꽃의 검 (45승 30패)
🎯 타겟 범위: +13 ~ +14

⚔️ #1: @상대유저 (+13) vs 나 (+12)
   → 승리! +125,000G 획득 (역배 성공!)

⚔️ #2: @다른유저 (+14) vs 나 (+12)
   → 패배... 다음 기회에

[30분 경과]
=== 배틀 종료 ===
⚔️ 총 배틀: 45회
🏆 승리: 18회 (40%)
💰 순수익: +2,350,000G
📤 통계 전송 완료!
```

---

## 3. 확장 데이터 수집 설계

### 3.1 수집 데이터 전체 목록

| 카테고리 | 데이터 | 용도 |
|----------|--------|------|
| **세션** | 시작/종료 시간, 모드, 실행 시간 | 사용 패턴 분석 |
| **강화** | 시도 횟수, 성공/실패/파괴, 레벨별 확률 | 최적 강화 전략 |
| **파밍** | 획득 검 등급, 히든 확률 | 파밍 효율 분석 |
| **판매** | 검 이름, 레벨, 판매가 | 가격 테이블 구축 |
| **배틀** | 상대 레벨, 승/패, 획득 골드 | 역배 전략 최적화 |
| **골드** | 수입/지출 내역 | 경제 시스템 분석 |

### 3.2 확장된 Stats 구조

```go
// internal/telemetry/telemetry.go

type Stats struct {
    // 기존
    TotalCycles      int `json:"total_cycles"`
    SuccessfulCycles int `json:"successful_cycles"`
    FailedCycles     int `json:"failed_cycles"`
    TotalGoldMined   int `json:"total_gold_mined"`
    TotalSwordsFound int `json:"total_swords_found"`
    SessionDuration  int `json:"session_duration_sec"`

    // 강화 통계 (NEW)
    EnhanceAttempts  int            `json:"enhance_attempts"`
    EnhanceSuccess   int            `json:"enhance_success"`
    EnhanceFail      int            `json:"enhance_fail"`
    EnhanceDestroy   int            `json:"enhance_destroy"`
    EnhanceByLevel   map[int]LevelStats `json:"enhance_by_level,omitempty"`

    // 배틀 통계 (NEW)
    BattleCount      int `json:"battle_count"`
    BattleWins       int `json:"battle_wins"`
    BattleLosses     int `json:"battle_losses"`
    BattleGoldEarned int `json:"battle_gold_earned"`
    UpsetWins        int `json:"upset_wins"`        // 역배 승리
    UpsetAttempts    int `json:"upset_attempts"`    // 역배 시도

    // 판매 통계 (NEW)
    SalesCount       int `json:"sales_count"`
    SalesTotalGold   int `json:"sales_total_gold"`
    SalesMaxPrice    int `json:"sales_max_price"`
    SalesAvgPrice    int `json:"sales_avg_price"`

    // 파밍 통계 (NEW)
    FarmingAttempts  int `json:"farming_attempts"`
    HiddenFound      int `json:"hidden_found"`
    TrashFound       int `json:"trash_found"`
}

type LevelStats struct {
    Attempts int `json:"attempts"`
    Success  int `json:"success"`
    Fail     int `json:"fail"`
    Destroy  int `json:"destroy"`
}
```

### 3.3 이벤트 로그 (상세 기록)

```go
// 개별 이벤트 상세 로그 (선택적 전송)
type EventLog struct {
    Timestamp   int64  `json:"ts"`
    EventType   string `json:"type"`    // enhance, battle, sale, farm
    Details     any    `json:"details"`
}

type EnhanceEvent struct {
    FromLevel int    `json:"from"`
    ToLevel   int    `json:"to"`
    Result    string `json:"result"`  // success, fail, destroy
}

type BattleEvent struct {
    MyLevel       int    `json:"my_level"`
    OpponentLevel int    `json:"opp_level"`
    Result        string `json:"result"`  // win, lose
    GoldEarned    int    `json:"gold"`
    IsUpset       bool   `json:"upset"`
}

type SaleEvent struct {
    SwordName  string `json:"sword_name"`
    Level      int    `json:"level"`
    Price      int    `json:"price"`
}
```

### 3.4 서버 DB 스키마 확장

```sql
-- 기존 telemetry 테이블 유지

-- 상세 이벤트 테이블 (NEW)
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- enhance, battle, sale, farm
    event_data JSON NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_events_session ON events(session_id);
CREATE INDEX idx_events_type ON events(event_type);

-- 집계 뷰 (분석용)
CREATE VIEW enhance_stats AS
SELECT
    json_extract(event_data, '$.from') as from_level,
    json_extract(event_data, '$.result') as result,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER (PARTITION BY json_extract(event_data, '$.from')), 2) as rate
FROM events
WHERE event_type = 'enhance'
GROUP BY from_level, result;

CREATE VIEW battle_upset_stats AS
SELECT
    json_extract(event_data, '$.my_level') as my_level,
    json_extract(event_data, '$.opp_level') - json_extract(event_data, '$.my_level') as level_diff,
    SUM(CASE WHEN json_extract(event_data, '$.result') = 'win' THEN 1 ELSE 0 END) as wins,
    COUNT(*) as total,
    ROUND(SUM(CASE WHEN json_extract(event_data, '$.result') = 'win' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as win_rate
FROM events
WHERE event_type = 'battle' AND json_extract(event_data, '$.upset') = 1
GROUP BY my_level, level_diff;

CREATE VIEW sale_price_table AS
SELECT
    json_extract(event_data, '$.level') as level,
    MIN(json_extract(event_data, '$.price')) as min_price,
    AVG(json_extract(event_data, '$.price')) as avg_price,
    MAX(json_extract(event_data, '$.price')) as max_price,
    COUNT(*) as sample_count
FROM events
WHERE event_type = 'sale'
GROUP BY level;
```

---

## 4. 데이터 활용 계획

### 4.1 분석 가능한 인사이트

| 분석 | 데이터 소스 | 기대 효과 |
|------|------------|----------|
| **강화 확률표** | enhance_by_level | 레벨별 실제 성공률 공개 |
| **역배 승률표** | battle_upset_stats | 최적 역배 레벨 차이 도출 |
| **판매 가격표** | sale_price_table | 검 레벨별 시세 공개 |
| **시간대별 효율** | session times | 최적 플레이 시간대 |
| **파밍 히든 확률** | farming stats | 히든 출현율 측정 |

### 4.2 커뮤니티 공개 API

```python
# 서버 API 확장

@app.get("/api/enhance-rates")
async def get_enhance_rates():
    """레벨별 강화 성공률 (커뮤니티 공개)"""
    # enhance_stats 뷰에서 조회

@app.get("/api/upset-rates")
async def get_upset_rates():
    """역배 승률표 (레벨 차이별)"""
    # battle_upset_stats 뷰에서 조회

@app.get("/api/price-table")
async def get_price_table():
    """검 레벨별 판매 시세"""
    # sale_price_table 뷰에서 조회
```

---

## 5. 구현 계획

### 5.1 Phase 1: 데이터 수집 확장 (1일)

```
[ ] Stats 구조체 확장 (enhance, battle, sale 필드)
[ ] RecordEnhance(), RecordBattle(), RecordSale() 메서드 추가
[ ] 기존 루프에 기록 호출 추가
[ ] 서버 DB 스키마 업데이트
```

### 5.2 Phase 2: 자동 배틀 모드 (2일)

```
[ ] OCR 파싱: 프로필, 랭킹, 배틀 결과
[ ] ModeBattle 상태 머신 구현
[ ] 타겟 선정 알고리즘 (레벨 차이 기반)
[ ] 배틀 쿨다운 관리
[ ] 메뉴 UI 추가
```

### 5.3 Phase 3: 분석 대시보드 (선택)

```
[ ] 통계 집계 API 구현
[ ] 간단한 웹 대시보드 (선택)
[ ] 주간 리포트 자동 생성
```

---

## 6. 기술 고려사항

### 6.1 Rate Limiting 대응

```go
const (
    battleCooldown = 5 * time.Second   // 배틀 간 최소 간격
    commandDelay   = 1 * time.Second   // 명령어 간 딜레이
)
```

### 6.2 OCR 정확도 개선

배틀 결과 메시지는 이모지가 많아 OCR 오인식 가능성 있음:
- `🏆결과` → 키워드 기반 파싱
- 골드 숫자에서 콤마 제거: `2,407,543` → `2407543`
- 유저명 `@` 기호로 구분

### 6.3 에러 복구

```go
func (e *Engine) loopBattle() {
    for e.running {
        // 프로필 읽기 실패 시 재시도
        profile, err := e.readProfile()
        if err != nil {
            logger.Warn("프로필 읽기 실패, 5초 후 재시도")
            time.Sleep(5 * time.Second)
            continue
        }

        // 랭킹에서 타겟 없으면 스킵
        targets := e.findTargets(profile.Level)
        if len(targets) == 0 {
            logger.Info("적합한 타겟 없음, 대기")
            time.Sleep(30 * time.Second)
            continue
        }

        // 배틀 실행
        ...
    }
}
```

---

## 7. 예상 데이터 수집량

| 사용자 수 | 일일 이벤트 | 월간 DB 증가 | 분석 가치 |
|----------|------------|-------------|----------|
| 10명 | ~1,000 | ~5MB | 기초 통계 |
| 100명 | ~10,000 | ~50MB | 신뢰할 수 있는 확률 |
| 1,000명 | ~100,000 | ~500MB | 정밀한 전략 도출 |

---

## 8. 리스크

| 리스크 | 대응 |
|--------|------|
| 배틀 쿨다운으로 인한 느린 진행 | 다른 활동(강화, 파밍)과 병행 |
| OCR 오인식 | 키워드 기반 파싱 + 검증 로직 |
| 게임 업데이트로 메시지 포맷 변경 | 파싱 로직 모듈화, 쉬운 수정 |
| 역배 연패로 골드 소진 | 최소 골드 보유 시 자동 중단 |

---

## 9. 결론

### 구현 우선순위

1. **높음**: 데이터 수집 확장 (강화/판매/파밍 상세 기록)
2. **높음**: 자동 배틀 기본 기능
3. **중간**: 역배 전략 최적화
4. **낮음**: 웹 대시보드

### 예상 효과

- **사용자**: 자동 배틀로 골드 수급 + 역배 스릴
- **데이터**: 게임 메커니즘 완전 해석 가능
- **커뮤니티**: 공개 확률표로 신뢰도 향상
