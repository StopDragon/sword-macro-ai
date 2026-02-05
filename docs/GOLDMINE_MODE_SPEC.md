# 골드 채굴 모드 (GoldMine Mode) 기획서

## 1. 모드 개요

### 목적
검 농사 → 강화 → 판매 사이클을 반복하여 골드 수익을 창출하는 자동화 모드.

### 기본 흐름
```
1. /수령 (새 검 획득)
2. /강화 반복 (목표 레벨까지)
3. /판매 (골드 획득)
4. 1번으로 돌아가 반복
```

### 핵심 지표
- **순수익**: 판매 수익 - 강화 비용
- **시간 효율**: 순수익 / 사이클 시간
- **최적 판매 레벨**: 시간 대비 수익이 가장 높은 레벨

---

## 2. 현재 문제점

### 2.1 하드코딩된 검 가격
현재 서버는 `defaultSwordPrices`를 그대로 반환:
```go
var defaultSwordPrices = []SwordPrice{
    {Level: 5, AvgPrice: 1250},    // 하드코딩
    {Level: 6, AvgPrice: 3000},    // 하드코딩
    {Level: 10, AvgPrice: 120000}, // 하드코딩
}
```
**증거**: 평균 가격이 모두 000/500으로 끝남 (실측 데이터라면 1,337 같은 자연수)

### 2.2 아이템 타입별 가격 구분 없음
- 일반(normal)과 특수(special) 아이템의 가격이 다를 수 있음
- 현재는 타입 구분 없이 레벨로만 집계

### 2.3 데이터 분산 (기존 방식)
기존 키 형식: `"{아이템명}_{레벨}"` (예: `"불꽃검_10"`, `"검_10"`)
- 같은 레벨이라도 아이템명마다 분산 → 샘플 수 부족

---

## 3. 개선 방향

### 3.1 아이템 타입 분류
| 타입 | 설명 | 예시 |
|------|------|------|
| `normal` | 일반 무기 | 몽둥이, 망치, 검, 칼, 도끼 |
| `special` | 특수 무기 | 칫솔, 우산, 단소, 젓가락, 광선검 |
| `trash` | 쓰레기 | 낡은 xx (0강) |

### 3.2 판매 통계 키 형식
**변경 전**: `"{아이템명}_{레벨}"` → `"불꽃검_10"`
**변경 후**: `"{타입}_{레벨}"` → `"normal_10"`, `"special_10"`

### 3.3 기대 효과
1. **빠른 데이터 수렴**: 모든 일반 10강이 하나로 집계
2. **타입별 가격 비교**: 일반 vs 특수 가격 차이 분석
3. **새로운 전략 가능**: 특수 아이템이 더 비싸면 "히든 파밍" 전략

---

## 4. 전체 데이터 흐름

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           골드 채굴 데이터 흐름                            │
└─────────────────────────────────────────────────────────────────────────┘

[클라이언트]                    [서버]                      [클라이언트]
     │                           │                              │
     │  1. 판매 발생              │                              │
     │  POST /api/telemetry      │                              │
     │  {                        │                              │
     │    "sword_sale_stats": {  │                              │
     │      "normal_10": {       │                              │
     │        total_price: 125000│                              │
     │        count: 1           │                              │
     │      }                    │                              │
     │    }                      │                              │
     │  }                        │                              │
     │                           │                              │
     │                     2. 저장 (SQLite)                      │
     │                     - key: "normal_10"                   │
     │                     - total_price += 125000              │
     │                     - count += 1                         │
     │                           │                              │
     │                     3. 통계 계산                          │
     │                     - 타입별 평균가격                      │
     │                     - 시간 효율 계산                       │
     │                           │                              │
     │                           │  4. 최적 레벨 요청            │
     │                           │  <─────────────────────────  │
     │                           │  GET /api/strategy/          │
     │                           │      optimal-sell-point      │
     │                           │                              │
     │                           │  5. 최적 레벨 응답            │
     │                           │  ─────────────────────────>  │
     │                           │  {                           │
     │                           │    optimal_level: 10,        │
     │                           │    by_type: {                │
     │                           │      normal: 10,             │
     │                           │      special: 12             │
     │                           │    }                         │
     │                           │  }                           │
     │                           │                              │
     │                           │              6. 목표 레벨 설정│
     │                           │              targetLevel = 10│
```

---

## 5. 클라이언트 → 서버 보고

### 5.1 텔레메트리 페이로드
```json
{
  "schema_version": 3,
  "session_id": "uuid-xxxx",
  "stats": {
    // 강화 통계 (타입+레벨별)
    "enhance_stats": {
      "normal_10": { "attempts": 5, "success": 1, "fail": 3, "destroy": 1 },
      "special_10": { "attempts": 3, "success": 1, "fail": 1, "destroy": 1 }
    },
    // 판매 통계 (타입+레벨별)
    "sword_sale_stats": {
      "normal_10": { "total_price": 125000, "count": 1 },
      "special_8": { "total_price": 45000, "count": 1 }
    }
  }
}
```

### 5.2 데이터 수집 시점

| 이벤트 | 기록 내용 | 키 형식 |
|--------|----------|---------|
| 강화 시도 | attempts++ | `{type}_{level}` |
| 강화 성공 | success++ | `{type}_{level}` |
| 강화 실패 (유지) | fail++ | `{type}_{level}` |
| 강화 파괴 | destroy++ | `{type}_{level}` |
| 판매 | total_price += price, count++ | `{type}_{level}` |

### 5.3 클라이언트 코드 (TODO)
```go
// telemetry.go - 강화 기록
func (t *Telemetry) RecordEnhanceWithType(itemType string, level int, result string) {
    key := fmt.Sprintf("%s_%d", itemType, level)  // "normal_10"
    if t.stats.EnhanceStats[key] == nil {
        t.stats.EnhanceStats[key] = &EnhanceStat{}
    }
    stat := t.stats.EnhanceStats[key]
    stat.Attempts++
    switch result {
    case "success":
        stat.Success++
    case "fail":
        stat.Fail++
    case "destroy":
        stat.Destroy++
    }
}

// telemetry.go - 판매 기록 (완료)
func (t *Telemetry) RecordSaleWithType(itemType string, level int, price int) {
    key := fmt.Sprintf("%s_%d", itemType, level)  // "normal_10"
    t.stats.SwordSaleStats[key].TotalPrice += price
    t.stats.SwordSaleStats[key].Count++
}
```

---

## 6. 서버 저장 로직

### 6.1 DB 스키마
```sql
-- 강화 통계 (타입+레벨별)
CREATE TABLE enhance_stats (
    key TEXT PRIMARY KEY,      -- "normal_10", "special_10"
    attempts INTEGER,          -- 시도 횟수
    success INTEGER,           -- 성공 횟수
    fail INTEGER,              -- 실패 횟수 (유지)
    destroy INTEGER            -- 파괴 횟수
);

-- 판매 통계 (타입+레벨별)
CREATE TABLE sword_sale_stats (
    key TEXT PRIMARY KEY,      -- "normal_10", "special_10"
    total_price INTEGER,       -- 총 판매 금액 (누적)
    count INTEGER              -- 판매 횟수 (누적)
);
```

### 6.2 텔레메트리 수신 핸들러
```go
func handleTelemetry(payload TelemetryPayload) {
    stats.mu.Lock()
    defer stats.mu.Unlock()

    // 강화 통계 누적 (타입+레벨별)
    for key, stat := range payload.Stats.EnhanceStats {
        if stats.enhanceStats[key] == nil {
            stats.enhanceStats[key] = &EnhanceStat{}
        }
        stats.enhanceStats[key].Attempts += stat.Attempts
        stats.enhanceStats[key].Success += stat.Success
        stats.enhanceStats[key].Fail += stat.Fail
        stats.enhanceStats[key].Destroy += stat.Destroy
    }

    // 판매 통계 누적 (타입+레벨별)
    for key, stat := range payload.Stats.SwordSaleStats {
        if stats.swordSaleStats[key] == nil {
            stats.swordSaleStats[key] = &SwordSaleStat{}
        }
        stats.swordSaleStats[key].TotalPrice += stat.TotalPrice
        stats.swordSaleStats[key].Count += stat.Count
    }

    // DB 저장 (비동기)
    go saveToDB()
}
```

### 6.3 DB 저장 (UPSERT)
```sql
-- 강화 통계
INSERT INTO enhance_stats (key, attempts, success, fail, destroy)
VALUES ('normal_10', 5, 1, 3, 1)
ON CONFLICT(key) DO UPDATE SET
    attempts = attempts + excluded.attempts,
    success = success + excluded.success,
    fail = fail + excluded.fail,
    destroy = destroy + excluded.destroy;

-- 판매 통계
INSERT INTO sword_sale_stats (key, total_price, count)
VALUES ('normal_10', 125000, 1)
ON CONFLICT(key) DO UPDATE SET
    total_price = total_price + excluded.total_price,
    count = count + excluded.count;
```

---

## 7. 통계 계산 로직

### 7.1 키 파싱 함수
```go
// "normal_10" → type="normal", level=10
// "불꽃검_10" → type="special", level=10 (폴백)
func extractTypeLevel(key string) (itemType string, level int) {
    parts := strings.Split(key, "_")
    levelStr := parts[len(parts)-1]
    level, _ = strconv.Atoi(levelStr)
    typePart := strings.Join(parts[:len(parts)-1], "_")

    // 신규 형식: "normal", "special", "trash"
    if typePart == "normal" || typePart == "special" || typePart == "trash" {
        return typePart, level
    }

    // 기존 형식 폴백: 아이템명 → 타입 변환
    return determineItemType(typePart), level
}
```

### 7.2 타입+레벨별 강화 확률 집계
```go
type EnhanceRateStat struct {
    Attempts    int
    Success     int
    Fail        int
    Destroy     int
    SuccessRate float64  // Success / Attempts * 100
    FailRate    float64  // Fail / Attempts * 100
    DestroyRate float64  // Destroy / Attempts * 100
    IsDefault   bool     // 샘플 부족 시 기본값 사용
}

func aggregateEnhanceByTypeLevel() map[string]map[int]EnhanceRateStat {
    result := make(map[string]map[int]EnhanceRateStat)
    // result["normal"][10] = {SuccessRate: 20.0, FailRate: 50.0, DestroyRate: 30.0}
    // result["special"][10] = {SuccessRate: 25.0, FailRate: 45.0, DestroyRate: 30.0}

    for key, stat := range stats.enhanceStats {
        itemType, level := extractTypeLevel(key)

        if result[itemType] == nil {
            result[itemType] = make(map[int]EnhanceRateStat)
        }

        entry := result[itemType][level]
        entry.Attempts += stat.Attempts
        entry.Success += stat.Success
        entry.Fail += stat.Fail
        entry.Destroy += stat.Destroy
        result[itemType][level] = entry
    }

    // 확률 계산
    for itemType, levels := range result {
        for level, stat := range levels {
            if stat.Attempts >= minSampleSize {
                total := float64(stat.Attempts)
                stat.SuccessRate = float64(stat.Success) / total * 100
                stat.FailRate = float64(stat.Fail) / total * 100
                stat.DestroyRate = float64(stat.Destroy) / total * 100
                stat.IsDefault = false
            } else {
                // 샘플 부족 → 기본값 사용
                defaultRate := getDefaultEnhanceRate(level)
                stat.SuccessRate = defaultRate.SuccessRate
                stat.FailRate = defaultRate.FailRate
                stat.DestroyRate = defaultRate.DestroyRate
                stat.IsDefault = true
            }
            result[itemType][level] = stat
        }
    }

    return result
}
```

### 7.3 타입+레벨별 평균 가격 집계
```go
type PriceStat struct {
    TotalPrice int
    Count      int
    AvgPrice   int
    IsDefault  bool  // 샘플 부족 시 기본값 사용
}

func aggregatePriceByTypeLevel() map[string]map[int]PriceStat {
    result := make(map[string]map[int]PriceStat)
    // result["normal"][10] = {AvgPrice: 125000, Count: 50}
    // result["special"][10] = {AvgPrice: 150000, Count: 20}

    for key, stat := range stats.swordSaleStats {
        itemType, level := extractTypeLevel(key)

        if result[itemType] == nil {
            result[itemType] = make(map[int]PriceStat)
        }

        entry := result[itemType][level]
        entry.TotalPrice += stat.TotalPrice
        entry.Count += stat.Count
        result[itemType][level] = entry
    }

    // 평균 계산
    for itemType, levels := range result {
        for level, stat := range levels {
            if stat.Count >= minSampleSize {
                stat.AvgPrice = stat.TotalPrice / stat.Count
                stat.IsDefault = false
            } else {
                stat.AvgPrice = getDefaultPrice(level)
                stat.IsDefault = true
            }
            result[itemType][level] = stat
        }
    }

    return result
}
```

### 7.4 샘플 수 기준
```go
const minSampleSize = 10  // 최소 10회 샘플 필요

// 샘플 부족 시 → 기본값 사용 (IsDefault = true)
// 샘플 충족 시 → 실측 평균 사용 (IsDefault = false)
```

---

## 8. 최적 판매 레벨 결정 알고리즘

### 8.1 핵심 공식
```
시간 효율 (G/분) = 평균가격 × 도달확률 / 예상시간(분)
```

### 8.2 변수 정의
| 변수 | 설명 | 소스 |
|------|------|------|
| `avgPrice[type][level]` | 타입별 레벨별 평균 판매가 | 실측 or 기본값 |
| `successRate[level]` | 레벨별 강화 성공률 | 실측 or 기본값 |
| `reachProb[level]` | 목표 레벨 도달 확률 | 계산 |
| `expectedTime[level]` | 예상 소요 시간 (초) | 계산 |

### 8.3 도달 확률 계산
```go
// 0강 → 목표 레벨까지 파괴 없이 도달할 확률
func calcReachProbability(targetLevel int) float64 {
    prob := 1.0
    for lvl := 0; lvl < targetLevel; lvl++ {
        rate := getEnhanceRate(lvl)  // 0.95, 0.90, ..., 0.05
        prob *= rate
    }
    return prob
}

// 예시: 10강 도달 확률
// = 0.95 × 0.90 × 0.85 × 0.80 × 0.70 × 0.60 × 0.50 × 0.40 × 0.30 × 0.20
// ≈ 0.15%
```

### 8.4 예상 시간 계산
```go
func calcExpectedTime(targetLevel int) float64 {
    const (
        farmTime    = 1.2  // 파밍 시간 (초)
        lowDelay    = 2.5  // 0-8강 강화 딜레이
        midDelay    = 3.5  // 9강 강화 딜레이
        highDelay   = 4.5  // 10강+ 강화 딜레이
    )

    totalTime := farmTime

    for lvl := 0; lvl < targetLevel; lvl++ {
        rate := getEnhanceRate(lvl)
        if rate <= 0 {
            continue
        }

        // 기대 시도 횟수 = 1 / 성공확률
        expectedTries := 1.0 / rate

        // 레벨별 딜레이
        var delay float64
        switch {
        case lvl >= 10:
            delay = highDelay
        case lvl >= 9:
            delay = midDelay
        default:
            delay = lowDelay
        }

        totalTime += expectedTries * delay
    }

    return totalTime
}
```

### 8.5 시간 효율 계산
```go
type LevelEfficiency struct {
    Level       int
    ItemType    string
    AvgPrice    int       // 평균 판매가
    ReachProb   float64   // 도달 확률
    ExpectedSec float64   // 예상 시간 (초)
    Efficiency  float64   // G/분
}

func calcEfficiency(itemType string, targetLevel int) LevelEfficiency {
    avgPrice := getAvgPrice(itemType, targetLevel)
    reachProb := calcReachProbability(targetLevel)
    expectedSec := calcExpectedTime(targetLevel)

    // 시간 효율 = (평균가격 × 도달확률) / (예상시간/60)
    efficiency := float64(avgPrice) * reachProb / (expectedSec / 60.0)

    return LevelEfficiency{
        Level:       targetLevel,
        ItemType:    itemType,
        AvgPrice:    avgPrice,
        ReachProb:   reachProb,
        ExpectedSec: expectedSec,
        Efficiency:  efficiency,
    }
}
```

### 8.6 최적 레벨 탐색
```go
func findOptimalLevel(itemType string) int {
    var maxEfficiency float64
    var optimalLevel int

    // 5강 ~ 15강 탐색
    for level := 5; level <= 15; level++ {
        eff := calcEfficiency(itemType, level)

        if eff.Efficiency > maxEfficiency {
            maxEfficiency = eff.Efficiency
            optimalLevel = level
        }
    }

    return optimalLevel
}
```

---

## 9. API 응답 설계

### 9.1 GET /api/game-data
```json
{
  "enhance_rates": [
    {
      "level": 10,
      "by_type": {
        "normal": {
          "success_rate": 20.0,
          "fail_rate": 50.0,
          "destroy_rate": 30.0,
          "attempts": 100,
          "is_default": false
        },
        "special": {
          "success_rate": 25.0,
          "fail_rate": 45.0,
          "destroy_rate": 30.0,
          "attempts": 50,
          "is_default": false
        }
      }
    }
  ],
  "sword_prices": [
    {
      "level": 10,
      "by_type": {
        "normal": { "avg": 125000, "count": 50, "is_default": false },
        "special": { "avg": 150000, "count": 20, "is_default": false }
      }
    }
  ],
  "battle_rewards": [...]
}
```

### 9.2 GET /api/strategy/optimal-sell-point
```json
{
  "optimal_level": 10,
  "optimal_by_type": {
    "normal": 10,
    "special": 12
  },
  "efficiency_table": [
    {
      "level": 5,
      "normal": { "avg_price": 1250, "efficiency": 52.1 },
      "special": { "avg_price": 1500, "efficiency": 62.5 }
    },
    {
      "level": 10,
      "normal": { "avg_price": 125000, "efficiency": 312.5, "is_optimal": true },
      "special": { "avg_price": 150000, "efficiency": 375.0 }
    },
    {
      "level": 12,
      "normal": { "avg_price": 900000, "efficiency": 225.0 },
      "special": { "avg_price": 1080000, "efficiency": 405.0, "is_optimal": true }
    }
  ],
  "recommendation": "special 아이템은 12강까지 강화 시 효율 8% 증가"
}
```

---

## 10. 활용 시나리오

### 10.1 기본 전략: 타입별 최적 레벨
```
📊 최적 판매 레벨 분석:

일반(normal):
- 최적 레벨: 10강
- 평균 판매가: 125,000G
- 시간 효율: 312.5 G/분

특수(special):
- 최적 레벨: 12강 (일반보다 2레벨 높음!)
- 평균 판매가: 1,080,000G
- 시간 효율: 405.0 G/분

💡 추천: 특수는 12강, 일반은 10강에 판매
```

### 10.2 히든 파밍 전략
특수 아이템이 일반보다 20% 비싸다면:
```
전략: 특수 나올 때까지 파밍
1. /수령
2. 일반이면 → /판매 (0강에서 바로 판매)
3. 특수면 → 12강까지 강화 후 판매
4. 반복

수익 분석:
- 특수 출현율: 10% (가정)
- 특수 12강 가격: 1,080,000G
- 일반 0강 가격: ~100G (손실)
- 기대 수익: 0.1 × 1,080,000 - 0.9 × 100 = 107,910G
```

### 10.3 클라이언트 UI 표시
```
📈 레벨별 시간 효율 (G/분):
┌──────┬─────────┬─────────┬──────────┐
│ 레벨 │  일반   │  특수   │   추천   │
├──────┼─────────┼─────────┼──────────┤
│  8강 │  156.3  │  187.5  │          │
│  9강 │  234.4  │  281.3  │          │
│ 10강 │  312.5  │  375.0  │ ✅ 일반  │
│ 11강 │  281.3  │  393.8  │          │
│ 12강 │  225.0  │  405.0  │ ✅ 특수  │
└──────┴─────────┴─────────┴──────────┘
```

---

## 11. 클라이언트 동적 목표 설정

### 11.1 흐름
```
┌─────────────────────────────────────────────────────────────┐
│                    동적 목표 레벨 설정                        │
└─────────────────────────────────────────────────────────────┘

1. 골드채굴 시작
   │
   ▼
2. 서버에서 타입별 최적 레벨 조회
   GET /api/strategy/optimal-sell-point
   → { "normal": 10, "special": 12 }
   │
   ▼
3. 사이클 시작
   │
   ├─→ 파밍 → "일반 검" 나옴
   │         → targetLevel = optimalLevels["normal"] = 10
   │         → 10강까지 강화 → 판매
   │
   ├─→ 파밍 → "특수 칫솔" 나옴
   │         → targetLevel = optimalLevels["special"] = 12
   │         → 12강까지 강화 → 판매
   │
   └─→ 파밍 → "쓰레기" 나옴
             → 바로 /판매 (강화 안함)
```

### 11.2 클라이언트 코드
```go
// data.go - 타입별 최적 레벨 조회
type OptimalLevels struct {
    Normal  int `json:"normal"`
    Special int `json:"special"`
}

func FetchOptimalLevels() (*OptimalLevels, error) {
    resp, err := http.Get(apiBaseURL + "/api/strategy/optimal-sell-point")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        OptimalByType OptimalLevels `json:"optimal_by_type"`
    }
    json.NewDecoder(resp.Body).Decode(&result)
    return &result.OptimalByType, nil
}
```

```go
// engine.go - 골드채굴 루프
func (e *Engine) loopGoldMine() {
    // 1. 시작 시 서버에서 타입별 최적 레벨 조회
    optimalLevels, err := FetchOptimalLevels()
    if err != nil {
        // 폴백: 기본값 사용
        optimalLevels = &OptimalLevels{Normal: 10, Special: 10}
    }
    fmt.Printf("📊 최적 판매 레벨: 일반=%d강, 특수=%d강\n",
        optimalLevels.Normal, optimalLevels.Special)

    for {
        // 2. 파밍
        itemName, itemType, itemLevel := e.farmForGoldMine()

        // 3. 타입에 따라 목표 레벨 동적 설정
        var targetLevel int
        switch itemType {
        case "special":
            targetLevel = optimalLevels.Special
        case "trash":
            targetLevel = 0  // 쓰레기는 바로 판매
        default:  // "normal"
            targetLevel = optimalLevels.Normal
        }

        // 4. 이미 목표 도달 또는 쓰레기면 바로 판매
        if itemLevel >= targetLevel {
            e.sendCommand("/판매")
            continue
        }

        // 5. 강화
        e.EnhanceToTarget(itemName, targetLevel)

        // 6. 판매
        e.sendCommand("/판매")
    }
}
```

### 11.3 UI 표시
```
📊 골드 채굴 모드 시작
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🎯 최적 판매 레벨 (서버 기준):
   - 일반(normal): 10강
   - 특수(special): 12강

📦 사이클 #1: 일반 검 → 목표 10강
⚔️ 강화 중... 0→1→2→3→4→5→6→7→8→9→10
💰 판매: +125,000G

📦 사이클 #2: 특수 칫솔 → 목표 12강 ← 타입에 따라 다름!
⚔️ 강화 중... 0→1→2→...→12
💰 판매: +1,080,000G

📦 사이클 #3: 쓰레기 → 바로 판매
💰 판매: +100G
```

### 11.4 폴백 처리
```go
// 서버 연결 실패 시 기본값 사용
var defaultOptimalLevels = OptimalLevels{
    Normal:  10,  // 기본 일반 목표
    Special: 10,  // 기본 특수 목표
}

// 주기적 갱신 (옵션)
// 30분마다 최적 레벨 재조회하여 실시간 반영
```

---

## 12. 구현 상태

### 클라이언트 - 판매 통계 (완료 ✅)
- [x] `telemetry.go`: `RecordSaleWithType()` 함수
- [x] `helpers.go`: `ReportGoldMineCycle(itemType, ...)` 시그니처
- [x] `engine.go`: `itemType` 전달
- [x] `engine.go`: loopSpecial 판매 통계 기록
- [x] `engine.go`: loopGoldMine 즉시 판매 통계 기록

### 클라이언트 - 강화 통계 (완료 ✅)
- [x] `telemetry.go`: `RecordEnhanceWithType()` 함수 추가
- [x] `engine.go`: loopEnhance 강화 시 타입 기록
- [x] `helpers.go`: EnhanceToTarget 강화 시 타입 기록

### 클라이언트 - 동적 목표 설정 (완료 ✅)
- [x] `data.go`: `TypeOptimal` 구조체 추가
- [x] `data.go`: `OptimalSellData.ByType` 필드 추가
- [x] `data.go`: `GetOptimalLevelByType()` 함수 추가
- [x] `data.go`: `GetOptimalLevelsByType()` 함수 추가
- [x] `engine.go`: loopGoldMine 시작 시 타입별 최적 레벨 조회
- [x] `engine.go`: loopGoldMine 타입별 목표 레벨 동적 적용
- [x] 폴백 처리 (서버 연결 실패 시 기본값 사용)

### 서버 (완료 ✅)
- [x] `extractTypeLevel()`: 키 파싱 함수
- [x] `handleTelemetry()`: SwordSaleStats/SwordEnhanceStats 수신
- [x] `handleOptimalSellPoint()`: 타입별 최적 레벨 계산 + by_type 응답
- [x] 타입별 강화 확률/판매 가격 집계

### 테스트
- [x] 클라이언트 빌드 테스트
- [x] 서버 빌드 테스트
- [ ] 강화 통계 타입별 전송 확인 (실행 테스트 필요)
- [ ] 판매 통계 타입별 전송 확인 (실행 테스트 필요)
- [ ] 타입별 확률/가격 집계 확인 (실행 테스트 필요)
- [ ] 동적 목표 설정 동작 확인 (실행 테스트 필요)

---

## 12. 타입별 데이터 요약

### 저장되는 데이터
| 데이터 | 키 형식 | 저장 내용 |
|--------|---------|----------|
| 강화 확률 | `{type}_{level}` | attempts, success, fail, destroy |
| 판매 가격 | `{type}_{level}` | total_price, count |

### 타입별 분석 가능 항목
| 분석 항목 | normal | special | trash |
|----------|--------|---------|-------|
| 강화 성공률 | ✅ | ✅ | ✅ |
| 강화 실패율 | ✅ | ✅ | ✅ |
| 파괴 확률 | ✅ | ✅ | ✅ |
| 평균 판매가 | ✅ | ✅ | ✅ |
| 최적 판매 레벨 | ✅ | ✅ | - |
| 시간 효율 | ✅ | ✅ | - |

### 전략적 활용
```
예시: 실측 데이터 분석 결과

10강 강화 확률:
- 일반(normal): 성공 20%, 실패 50%, 파괴 30%
- 특수(special): 성공 25%, 실패 45%, 파괴 30%  ← 5% 높음!

10강 판매 가격:
- 일반(normal): 125,000G
- 특수(special): 150,000G  ← 20% 높음!

→ 특수 아이템은 강화 확률도 높고 가격도 높음
→ 특수만 강화하는 "히든 파밍" 전략이 효율적
```

---

## 변경 이력

| 날짜 | 버전 | 내용 |
|------|------|------|
| 2026-02-05 | v1.0 | 초안 작성 |
| 2026-02-05 | v1.1 | 전체 데이터 흐름 및 알고리즘 상세화 |
| 2026-02-05 | v1.2 | 강화 통계도 타입별 저장 추가 |
