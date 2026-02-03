# sword-macro-ai 텔레메트리 시스템 기획서

> **문서 버전**: 2.0 (자체 서버 버전)
> **작성일**: 2026-02-03
> **목적**: 오픈소스 사용자 데이터 수집을 통한 매크로 최적화

---

## 1. Executive Summary

### 목표
사용자들의 **익명화된 게임 데이터**를 수집하여 강화 확률 테이블, 최적 딜레이 값, 골드 채굴 효율 등을 **빅데이터 기반으로 검증 및 개선**한다.

### 핵심 결정사항

| 항목 | 결정 | 근거 |
|------|------|------|
| **수집 방식** | Opt-out (기본 활성화 + 쉬운 비활성화) | 3% 미만의 opt-in 참여율은 통계적으로 무의미 |
| **서버 인프라** | **자체 Proxmox LXC** (FastAPI + SQLite) | 월 $0, 무제한 데이터, 완전 제어 |
| **데이터 형식** | JSON 배치 업로드 (일 1회) | 대역폭 최소화, 오프라인 지원 |
| **한국 법률 대응** | 완전 익명화로 PIPA 적용 제외 | 개인정보에 해당하지 않도록 설계 |

### 예상 수집 규모

| 시나리오 | 일일 사용자 | 월간 이벤트 | 서버 비용 |
|----------|------------|-------------|----------|
| 초기 (1-3개월) | 10-50명 | ~15만 건 | **$0** (전기세만) |
| 성장기 (6개월) | 100-500명 | ~150만 건 | **$0** |
| 성숙기 (1년+) | 1000명+ | ~3000만 건 | **$0** |

### 자체 서버 vs 클라우드 비교

| 항목 | 클라우드 (Supabase+Vercel) | 자체 Proxmox |
|------|---------------------------|--------------|
| 월 비용 | $0-45 | **$0** |
| 데이터 한도 | 500MB / 50K MAU | **무제한** |
| 제어권 | 제한적 | **완전 제어** |
| 의존성 | 외부 서비스 종속 | **없음** |
| 운영 복잡도 | 낮음 | 중간 |
| 데이터 주권 | 해외 서버 | **국내 자체 보관** |

---

## 2. 수집할 데이터

### 2.1 현재 로컬 CSV 필드 (기존)

```
timestamp, event, level, result, gold, item, mode, cycle_id, cycle_sec, gold_earned
```

### 2.2 텔레메트리 전송 데이터 (신규 설계)

**수집 O (익명화된 집계 데이터)**:

| 필드 | 설명 | 예시 |
|------|------|------|
| `app_version` | 앱 버전 | `"1.2.3"` |
| `os_type` | 운영체제 종류 | `"windows"` / `"darwin"` |
| `session_id` | 세션별 랜덤 UUID | `"a1b2c3d4..."` |
| `period` | 집계 기간 (일 단위) | `"2026-02-03"` |
| `enhance_counts` | 레벨별 강화 시도 횟수 | `{"+1": 50, "+5": 30, "+10": 12}` |
| `enhance_results` | 레벨별 결과 분포 | `{"+5": {"success": 15, "hold": 10, "destroy": 5}}` |
| `farm_stats` | 파밍 통계 | `{"trash": 120, "hidden": 15}` |
| `cycle_stats` | 사이클 집계 | `{"count": 8, "avg_sec": 62.5, "avg_gold": 71000}` |
| `gold_per_hour` | 시간당 골드 (구간화) | `"1M-1.5M"` |
| `error_types` | 오류 유형 카운트 | `{"ocr_fail": 3, "input_fail": 1}` |

**수집 X (절대 수집 안 함)**:

| 항목 | 이유 |
|------|------|
| IP 주소 | 서버에서 즉시 폐기 |
| 정확한 타임스탬프 | 일 단위로 버킷화 |
| 게임 계정 정보 | 아이템 이름 포함 안 함 |
| 파일 경로 | 수집하지 않음 |
| 기기 고유 ID | 세션 UUID만 사용 (재시작 시 변경) |
| 골드 정확한 금액 | 구간으로만 전송 ("100K-500K") |

### 2.3 데이터 구조 예시

```json
{
  "schema_version": 1,
  "app_version": "1.2.3",
  "os_type": "windows",
  "session_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "period": "2026-02-03",
  "stats": {
    "enhance": {
      "by_level": {
        "1": {"attempts": 45, "success": 42, "hold": 2, "destroy": 1},
        "5": {"attempts": 30, "success": 15, "hold": 10, "destroy": 5},
        "10": {"attempts": 8, "success": 2, "hold": 4, "destroy": 2}
      }
    },
    "farm": {
      "trash_count": 85,
      "hidden_count": 10
    },
    "cycles": {
      "completed": 8,
      "avg_duration_sec": 62.5,
      "avg_gold_bucket": "50K-100K"
    },
    "session": {
      "duration_bucket": "1-2h",
      "gold_per_hour_bucket": "1M-1.5M"
    },
    "errors": {
      "ocr_empty": 12,
      "input_fail": 2
    }
  }
}
```

---

## 3. 시스템 아키텍처 (자체 서버)

### 3.1 전체 흐름

```
┌─────────────────────────────────────────────────────────────────┐
│                        사용자 PC (클라이언트)                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  sword_macro.py                                          │   │
│  │      │                                                   │   │
│  │      ├─→ 기존: sword_data.csv (로컬 저장, 그대로 유지)      │   │
│  │      │                                                   │   │
│  │      └─→ 신규: telemetry.py (메모리/로컬 집계)             │   │
│  │               │                                          │   │
│  │               └─→ 세션 종료 시 또는 24시간마다 전송          │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTPS POST (gzip 압축)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              Proxmox LXC 컨테이너 (자체 서버)                     │
│              최소 사양: 1 vCPU, 512MB RAM, 5GB 디스크             │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Caddy (리버스 프록시 + 자동 HTTPS)                        │   │
│  │      │                                                   │   │
│  │      ▼                                                   │   │
│  │  FastAPI (Python)                                        │   │
│  │  - POST /api/telemetry (데이터 수신)                       │   │
│  │  - GET /api/stats (공개 통계 조회)                         │   │
│  │  - IP 주소 비저장                                         │   │
│  │  - Rate limiting (10회/분)                                │   │
│  │  - 스키마 검증                                            │   │
│  │      │                                                   │   │
│  │      ▼                                                   │   │
│  │  SQLite DB (/data/telemetry.db)                          │   │
│  │  - 단일 파일, 백업 용이                                    │   │
│  │  - 수백만 행까지 문제없음                                   │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Grafana (선택, 별도 LXC 또는 같은 컨테이너)                 │   │
│  │  - SQLite 데이터 시각화                                    │   │
│  │  - 강화 확률 대시보드                                      │   │
│  │  - G/h 트렌드 차트                                        │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 클라이언트 구현 (신규 모듈)

**`telemetry.py`** (새 파일):

```python
# telemetry.py - 익명 사용 통계 수집
import json
import os
import uuid
import gzip
import urllib.request
from datetime import date

TELEMETRY_ENDPOINT = "https://telemetry.yourdomain.com/api/telemetry"  # 자체 서버 주소
TELEMETRY_FILE = os.path.join(os.path.dirname(__file__), '.telemetry_state.json')

class Telemetry:
    def __init__(self, enabled=True):
        self.enabled = enabled
        self.session_id = str(uuid.uuid4())
        self.stats = self._init_stats()
        self._load_state()

    def _init_stats(self):
        return {
            'enhance': {'by_level': {}},
            'farm': {'trash': 0, 'hidden': 0},
            'cycles': {'count': 0, 'total_sec': 0, 'total_gold': 0},
            'errors': {}
        }

    def record_enhance(self, level, result):
        if not self.enabled:
            return
        key = str(level)
        if key not in self.stats['enhance']['by_level']:
            self.stats['enhance']['by_level'][key] = {'attempts': 0, 'success': 0, 'hold': 0, 'destroy': 0}
        self.stats['enhance']['by_level'][key]['attempts'] += 1
        self.stats['enhance']['by_level'][key][result] += 1

    # ... 나머지 메서드들

    def flush(self):
        """서버로 전송 후 로컬 상태 초기화"""
        if not self.enabled or not self._has_data():
            return

        payload = {
            'schema_version': 1,
            'app_version': VERSION,
            'os_type': 'darwin' if sys.platform == 'darwin' else 'windows',
            'session_id': self.session_id,
            'period': date.today().isoformat(),
            'stats': self.stats
        }

        try:
            data = gzip.compress(json.dumps(payload).encode())
            req = urllib.request.Request(
                TELEMETRY_ENDPOINT,
                data=data,
                headers={'Content-Type': 'application/json', 'Content-Encoding': 'gzip'}
            )
            urllib.request.urlopen(req, timeout=5)
            self.stats = self._init_stats()
        except Exception:
            pass  # 실패해도 무시 (사용자 경험 우선)
```

### 3.3 서버 구현 (FastAPI + SQLite)

**`server/main.py`**:

```python
# FastAPI 텔레메트리 수신 서버
from fastapi import FastAPI, Request, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import Optional, Dict, Any
import sqlite3
import json
import gzip
from datetime import datetime
from slowapi import Limiter
from slowapi.util import get_remote_address

app = FastAPI(title="sword-macro Telemetry Server")
limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter

# CORS 설정
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["POST", "GET"],
    allow_headers=["*"],
)

# SQLite 초기화
def init_db():
    conn = sqlite3.connect('/data/telemetry.db')
    conn.execute('''
        CREATE TABLE IF NOT EXISTS telemetry_events (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            received_at TEXT DEFAULT CURRENT_TIMESTAMP,
            app_version TEXT,
            os_type TEXT,
            period TEXT,
            stats JSON
        )
    ''')
    conn.execute('''
        CREATE INDEX IF NOT EXISTS idx_period ON telemetry_events(period)
    ''')
    conn.commit()
    conn.close()

init_db()

class TelemetryPayload(BaseModel):
    schema_version: int
    app_version: str
    os_type: str
    session_id: str
    period: str
    stats: Dict[str, Any]

@app.post("/api/telemetry")
@limiter.limit("10/minute")  # Rate limiting: 분당 10회
async def receive_telemetry(request: Request, payload: TelemetryPayload):
    # IP 주소는 의도적으로 저장하지 않음 (로깅도 안 함)

    # 스키마 검증
    if payload.schema_version != 1:
        raise HTTPException(status_code=400, detail="Unsupported schema version")

    # SQLite에 저장
    conn = sqlite3.connect('/data/telemetry.db')
    conn.execute(
        '''INSERT INTO telemetry_events (app_version, os_type, period, stats)
           VALUES (?, ?, ?, ?)''',
        (payload.app_version, payload.os_type, payload.period, json.dumps(payload.stats))
    )
    conn.commit()
    conn.close()

    return {"status": "ok"}

@app.get("/api/stats")
async def get_public_stats():
    """공개 통계 API (커뮤니티 공유용)"""
    conn = sqlite3.connect('/data/telemetry.db')
    conn.row_factory = sqlite3.Row

    # 전체 통계 집계
    result = conn.execute('''
        SELECT
            COUNT(*) as total_sessions,
            COUNT(DISTINCT period) as days_collected
        FROM telemetry_events
    ''').fetchone()

    conn.close()
    return {
        "total_sessions": result["total_sessions"],
        "days_collected": result["days_collected"],
        "last_updated": datetime.now().isoformat()
    }

@app.get("/health")
async def health_check():
    return {"status": "healthy"}
```

**`server/requirements.txt`**:

```
fastapi==0.109.0
uvicorn[standard]==0.27.0
pydantic==2.5.3
slowapi==0.1.9
```

### 3.4 Proxmox 서버 설정 가이드

#### Step 1: LXC 컨테이너 생성

```bash
# Proxmox 웹 UI 또는 CLI에서 실행
# Debian 12 또는 Ubuntu 22.04 템플릿 권장

# CLI 예시:
pct create 200 local:vztmpl/debian-12-standard_12.2-1_amd64.tar.zst \
  --hostname telemetry \
  --memory 512 \
  --cores 1 \
  --rootfs local-lvm:5 \
  --net0 name=eth0,bridge=vmbr0,ip=dhcp \
  --unprivileged 1

pct start 200
pct enter 200
```

#### Step 2: 기본 패키지 설치

```bash
# 컨테이너 내부에서 실행
apt update && apt upgrade -y
apt install -y python3 python3-pip python3-venv curl debian-keyring debian-archive-keyring apt-transport-https

# 데이터 디렉토리 생성
mkdir -p /data /opt/telemetry
```

#### Step 3: FastAPI 서버 설정

```bash
# Python 가상환경 생성
cd /opt/telemetry
python3 -m venv venv
source venv/bin/activate

# 의존성 설치
pip install fastapi uvicorn[standard] pydantic slowapi

# main.py 생성 (위 3.3절 코드 복사)
nano main.py
```

#### Step 4: systemd 서비스 등록

```bash
# /etc/systemd/system/telemetry.service 생성
cat > /etc/systemd/system/telemetry.service << 'EOF'
[Unit]
Description=Sword Macro Telemetry Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/telemetry
ExecStart=/opt/telemetry/venv/bin/uvicorn main:app --host 127.0.0.1 --port 8000
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 서비스 활성화 및 시작
systemctl daemon-reload
systemctl enable telemetry
systemctl start telemetry

# 상태 확인
systemctl status telemetry
```

#### Step 5: Caddy 리버스 프록시 (자동 HTTPS)

```bash
# Caddy 설치 (Debian/Ubuntu)
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
apt update
apt install caddy

# Caddyfile 설정
cat > /etc/caddy/Caddyfile << 'EOF'
telemetry.yourdomain.com {
    reverse_proxy localhost:8000

    # 보안 헤더
    header {
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
    }

    # 로그 (IP 제외)
    log {
        output file /var/log/caddy/telemetry.log
        format json
    }
}
EOF

# Caddy 재시작 (자동으로 Let's Encrypt 인증서 발급)
systemctl restart caddy
```

#### Step 6: 방화벽 설정 (Proxmox 호스트)

```bash
# Proxmox 호스트에서 실행
# 포트 포워딩 (외부 443 → LXC 443)

# iptables 예시 (또는 Proxmox 웹 UI의 Firewall 설정)
iptables -t nat -A PREROUTING -i vmbr0 -p tcp --dport 443 -j DNAT --to 192.168.1.200:443
iptables -A FORWARD -p tcp -d 192.168.1.200 --dport 443 -j ACCEPT

# 또는 Proxmox Firewall 설정 파일
# /etc/pve/firewall/cluster.fw 에 추가
```

#### Step 7: DNS 설정

```
; 도메인 레지스트라 또는 DNS 서버에서 설정
telemetry.yourdomain.com.  A  [Proxmox 서버 공인 IP]

; 또는 DDNS 서비스 사용 (가정용 유동 IP인 경우)
```

#### Step 8: 테스트

```bash
# 로컬 테스트
curl http://localhost:8000/health
# 예상 응답: {"status":"healthy"}

# 외부 테스트 (DNS 전파 후)
curl https://telemetry.yourdomain.com/health
# 예상 응답: {"status":"healthy"}

# 텔레메트리 전송 테스트
curl -X POST https://telemetry.yourdomain.com/api/telemetry \
  -H "Content-Type: application/json" \
  -d '{"schema_version":1,"app_version":"1.0.0","os_type":"test","session_id":"test-123","period":"2026-02-03","stats":{}}'
# 예상 응답: {"status":"ok"}
```

#### 유지보수 명령어

```bash
# 로그 확인
journalctl -u telemetry -f

# 서비스 재시작
systemctl restart telemetry

# DB 백업 (cron으로 자동화 권장)
cp /data/telemetry.db /backup/telemetry_$(date +%Y%m%d).db

# DB 크기 확인
ls -lh /data/telemetry.db

# SQLite 직접 조회
sqlite3 /data/telemetry.db "SELECT COUNT(*) FROM telemetry_events;"
```

---

## 4. 동의 및 개인정보 보호

### 4.1 동의 UX 설계

**첫 실행 시 표시되는 메시지**:

```
==================================================
  sword-macro-ai — 검키우기 자동화 + AI 데이터 분석
==================================================

  [익명 사용 통계 안내]

  앱 개선을 위해 익명의 사용 데이터를 수집합니다.

  수집 항목:
  - 운영체제 종류, 앱 버전
  - 강화 레벨별 시도 횟수 및 결과
  - 사이클 소요 시간 (구간화)

  수집하지 않는 항목:
  - 게임 계정 정보, IP 주소, 기기 식별자

  언제든 설정에서 비활성화할 수 있습니다.

  [Enter] 계속  |  [T] 텔레메트리 끄고 계속
==================================================
```

**설정 메뉴 추가**:

```
[옵션 설정]
1. 감속 시작 레벨 (9강)
2. 일반 속도 (2.5초)
...
7. 좌표 고정 (OFF)
8. 좌표 직접 입력
9. 익명 통계 수집 (ON)   ← 신규
0. 뒤로 가기
```

### 4.2 법률 준수

| 법률 | 적용 여부 | 근거 |
|------|----------|------|
| **한국 PIPA** | 적용 제외 | 완전 익명화 데이터는 개인정보에 해당하지 않음 |
| **GDPR** | 적용 제외 | 개인 식별 불가능한 집계 데이터 |
| **CCPA** | 적용 제외 | "Personal Information" 정의에 해당 안 함 |

**익명화 보장 방법**:
1. IP 주소: 서버에서 절대 저장/로깅 안 함
2. 타임스탬프: 일(day) 단위로만 기록
3. 골드/시간: 구간(bucket)으로 변환 ("1M-1.5M G/h")
4. 세션 ID: 앱 재시작마다 새로 생성 (추적 불가)

### 4.3 한국 사용자 특별 처리

한국 `locale`을 감지하면 **opt-in 방식**으로 전환:

```python
import locale

if locale.getdefaultlocale()[0] and locale.getdefaultlocale()[0].startswith('ko'):
    # 한국어 사용자: 명시적 동의 필요
    print("  [ ] 익명 데이터 전송에 동의합니다 (선택)")
    consent = input("동의하시면 Y 입력: ").strip().lower() == 'y'
    telemetry_enabled = consent
else:
    # 기타: opt-out (기본 활성화)
    telemetry_enabled = True
```

---

## 5. 비용 분석 (자체 서버)

### 5.1 Proxmox LXC 리소스 요구사항

| 리소스 | 최소 | 권장 | 비고 |
|--------|------|------|------|
| **vCPU** | 1 | 2 | FastAPI 단일 워커 |
| **RAM** | 512MB | 1GB | SQLite 캐시 포함 |
| **디스크** | 5GB | 20GB | 수년간 데이터 보관 가능 |
| **네트워크** | 포트 443 오픈 | - | Caddy 자동 HTTPS |

### 5.2 예상 비용

| 항목 | 비용 | 비고 |
|------|------|------|
| **서버 비용** | **$0** | 기존 Proxmox 활용 |
| **도메인** | $0-12/년 | 기존 도메인 서브도메인 사용 가능 |
| **SSL 인증서** | $0 | Caddy 자동 Let's Encrypt |
| **데이터 한도** | 무제한 | 로컬 디스크 한도까지 |
| **대역폭** | $0 | 가정용 인터넷 활용 |

### 5.3 성장 시나리오

| 월간 사용자 | 예상 DB 크기 | 일일 요청 | 서버 비용 |
|------------|-------------|----------|----------|
| ~100명 | ~10MB/월 | ~100회 | **$0** |
| ~1,000명 | ~100MB/월 | ~1K회 | **$0** |
| ~10,000명 | ~1GB/월 | ~10K회 | **$0** |
| ~50,000명 | ~5GB/월 | ~50K회 | **$0** (디스크 확장 필요) |

### 5.4 클라우드 대비 절감액

| 사용 규모 | 클라우드 비용 | 자체 서버 | 연간 절감 |
|----------|-------------|----------|----------|
| 소규모 (100명) | $0 | $0 | $0 |
| 중규모 (1,000명) | $0-25/월 | $0 | $0-300/년 |
| 대규모 (10,000명) | $45-100/월 | $0 | **$540-1,200/년** |

---

## 6. 개발 로드맵

### Phase 1: 서버 인프라 구축 (1일)

| 작업 | 예상 시간 | 담당 |
|------|----------|------|
| Proxmox LXC 컨테이너 생성 | 15분 | 인프라 |
| Python + FastAPI 설치 | 15분 | 인프라 |
| Caddy 리버스 프록시 설정 | 30분 | 인프라 |
| 도메인 + DNS 설정 | 15분 | 인프라 |
| systemd 서비스 등록 | 15분 | 인프라 |
| **합계** | **~1.5시간** | |

### Phase 2: 클라이언트 개발 (1일)

| 작업 | 예상 시간 | 담당 |
|------|----------|------|
| `telemetry.py` 모듈 작성 | 4시간 | 개발 |
| sword_macro.py 통합 | 2시간 | 개발 |
| 동의 UI 구현 | 2시간 | 개발 |
| 테스트 및 디버깅 | 3시간 | QA |
| **합계** | **~11시간** | |

### Phase 3: 분석 대시보드 (선택, 1주)

| 작업 | 예상 시간 |
|------|----------|
| Grafana LXC 설정 (선택) | 1시간 |
| SQLite 데이터 시각화 쿼리 | 4시간 |
| Jupyter Notebook 분석 템플릿 | 3시간 |
| 자동 리포트 생성 스크립트 | 4시간 |

### Phase 4: 피드백 루프 (지속)

| 활동 | 주기 |
|------|------|
| 데이터 리뷰 및 인사이트 도출 | 주 1회 |
| 딜레이 파라미터 최적화 | 월 1회 |
| 커뮤니티 분석 결과 공유 | 분기 1회 |
| SQLite 백업 | 주 1회 (자동화)

---

## 7. 수집 데이터 활용 계획

### 7.1 즉시 활용 가능

| 분석 목표 | 필요 데이터 | 기대 효과 |
|----------|------------|----------|
| **레벨별 성공률 테이블** | enhance by_level | 체감 확률 vs 실제 확률 검증 |
| **최적 딜레이 튜닝** | cycle_stats, errors | OCR 인식률 vs 속도 트레이드오프 최적화 |
| **ROI 최적 목표 레벨** | enhance + cycle_stats | +10 vs +9 vs +8 수익성 비교 |
| **히든 드랍률** | farm stats | 파밍 전략 최적화 |

### 7.2 장기 활용

| 분석 목표 | 필요 규모 | 예상 시기 |
|----------|----------|----------|
| 시간대별 성공률 차이 | ~10K 세션 | 3-6개월 |
| OS별 성능 차이 (Vision vs EasyOCR) | ~5K 세션 | 1-3개월 |
| 연속 실패/성공 패턴 분석 | ~50K 이벤트 | 6개월+ |

### 7.3 커뮤니티 환원

수집된 데이터로 도출한 인사이트는 **README, 블로그, 또는 GitHub Discussions**에 공개:

```markdown
## 커뮤니티 데이터 분석 결과 (2026년 2월)

1,234세션, 45,678회 강화 데이터 분석 결과:

| 강화 레벨 | 성공률 | 유지율 | 파괴율 |
|----------|-------|-------|-------|
| +1 → +2 | 91.2% | 6.3% | 2.5% |
| +5 → +6 | 49.8% | 34.1% | 16.1% |
| +9 → +10 | 31.4% | 42.3% | 26.3% |

> 데이터 출처: sword-macro-ai 익명 텔레메트리 (opt-out 가능)
```

---

## 8. 리스크 및 대응

| 리스크 | 심각도 | 대응 방안 |
|--------|-------|----------|
| 사용자 반발 (프라이버시 우려) | 중 | 투명한 문서화, 쉬운 opt-out, 수집 데이터 공개 |
| 서버 다운타임 | 중 | systemd 자동 재시작, 헬스체크 모니터링 |
| 가정용 IP 변경 (DDNS) | 하 | DDNS 서비스 사용 또는 고정 IP 설정 |
| 게임사 제재 | 중 | 텔레메트리와 매크로 기능은 별개, 단순 통계일 뿐 |
| 데이터 유출 | 하 | 익명화로 유출되어도 피해 없음 |
| 디스크 용량 부족 | 하 | 주기적 모니터링, 오래된 데이터 아카이빙 |
| SSL 인증서 만료 | 하 | Caddy 자동 갱신 (수동 개입 불필요) |

---

## 9. 결론 및 권장사항

### 권장 아키텍처 (자체 서버)
- **클라이언트**: Python `telemetry.py` 모듈 (일 배치 전송)
- **서버**: Proxmox LXC + Caddy + FastAPI + SQLite
- **비용**: **$0/월** (전기세만, 무제한 확장)
- **동의**: Opt-out (한국 사용자만 Opt-in)
- **데이터 주권**: 국내 자체 보관, 완전 제어

### 자체 서버의 장점

| 항목 | 클라우드 | 자체 Proxmox |
|------|---------|-------------|
| 월 비용 | $0-45 (성장 시 증가) | **$0 (고정)** |
| 데이터 한도 | 제한적 | **디스크 한도까지 무제한** |
| 외부 의존성 | 서비스 종속 | **없음** |
| 학습 가치 | 낮음 | **높음** (인프라 경험) |
| 유연성 | 플랫폼 제한 | **완전 자유** |

### 즉시 실행 가능한 다음 단계

1. **Proxmox LXC 생성** (15분)
   - Debian 12 템플릿, 512MB RAM, 5GB 디스크
2. **FastAPI 서버 배포** (30분)
   - Python + uvicorn + SQLite
3. **Caddy 리버스 프록시** (30분)
   - 자동 HTTPS, Let's Encrypt
4. **도메인 설정** (15분)
   - 서브도메인 A 레코드 추가
5. **클라이언트 개발** (4시간)
   - telemetry.py 모듈 + sword_macro.py 통합
6. **README 업데이트** (30분)
   - 프라이버시 정책 추가

### 예상 ROI

| 투자 | 기대 효과 |
|------|----------|
| 서버 설정 ~2시간 | 완전 자체 인프라 구축 |
| 클라이언트 개발 ~12시간 | 실제 사용 데이터 기반 최적화 |
| 서버 비용 **$0/월** | 무제한 데이터, 장기 운영 가능 |
| 유지보수 주 30분 | 지속적 개선 사이클 구축 |
| 인프라 경험 | 향후 프로젝트에 재사용 가능 |

---

## 부록: 참고 자료

### 오픈소스 텔레메트리 사례
- [Homebrew Analytics 정책](https://docs.brew.sh/Analytics)
- [VS Code Telemetry 문서](https://code.visualstudio.com/docs/getstarted/telemetry)
- [1984 Ventures: Open Source Telemetry](https://1984.vc/docs/founders-handbook/eng/open-source-telemetry/)

### 법률 및 개인정보
- [한국 PIPA 2025 업데이트](https://crossborderadvisorysolutions.com/personal-information-protection-act-pipa-updates-2025/)

### 자체 호스팅 가이드
- [FastAPI 공식 문서](https://fastapi.tiangolo.com/)
- [Caddy 리버스 프록시 설정](https://caddyserver.com/docs/quick-starts/reverse-proxy)
- [Proxmox LXC 문서](https://pve.proxmox.com/wiki/Linux_Container)
- [SQLite 대용량 데이터 최적화](https://www.sqlite.org/whentouse.html)
- [Let's Encrypt with Caddy](https://caddyserver.com/docs/automatic-https)
