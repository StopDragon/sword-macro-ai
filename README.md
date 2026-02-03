# sword-macro-ai

카카오톡 **검키우기** 게임 자동화 매크로

> Go 네이티브 빌드 — 바이너리 2.3MB, 외부 의존성 없음

## 다운로드

### [최신 버전 다운로드](../../releases/latest)

| 플랫폼 | 파일 | 설치 |
|--------|------|------|
| macOS | `SwordMacro-macOS.zip` | 압축 해제 → 실행 |
| Windows | `SwordMacro-Windows.zip` | 압축 해제 → exe 실행 |

<details>
<summary>설치 가이드</summary>

**macOS**
1. 압축 해제
2. 첫 실행: 우클릭 → 열기 (개발자 확인 우회)
3. 시스템 설정 → 개인정보 보호 → **접근성** 권한 허용
4. 시스템 설정 → 개인정보 보호 → **화면 기록** 권한 허용

**Windows**
1. 압축 해제 → exe 실행
2. Defender 경고: 추가 정보 → 실행
3. 한국어 OCR 필요: 설정 → 시간 및 언어 → 언어 → 한국어 추가

</details>

## 기능

| 모드 | 설명 |
|------|------|
| 강화 목표 달성 | 설정한 레벨까지 자동 강화 |
| 히든 검 뽑기 | 히든 아이템 발견까지 파밍 |
| 골드 채굴 | 파밍 → 강화 → 판매 무한 반복 |
| 자동 배틀 (역배) | 높은 레벨 상대와 자동 대결 |
| 내 프로필 분석 | 검 정보, 판매가, 강화확률, 역배 기대값 분석 |

**조작**: F8 일시정지, F9 재시작, 마우스 좌상단 = 비상정지

## 사용법

1. 카카오톡에서 검키우기 채팅방 열기
2. 매크로 실행 → 모드 선택
3. 카카오톡 **메시지 입력칸**에 마우스 올리기 → 3초 후 좌표 자동 저장
4. 매크로 시작

## 설정

| 항목 | 기본값 | 설명 |
|------|--------|------|
| 감속 시작 레벨 | +9 | 고강 딜레이 시작점 |
| 중간 속도 | 2.5초 | +5~+8 강화 대기 |
| 고강 속도 | 3.5초 | +9~ 강화 대기 |
| 좌표 고정 | OFF | 저장된 좌표 재사용 |

설정 파일: `sword_config.json`

## 빌드

```bash
git clone https://github.com/StopDragon/sword-macro-ai.git
cd sword-macro-ai

# 클라이언트 (매크로)
make build-mac           # macOS
make build-mac-universal # macOS Universal (Intel + Apple Silicon)
make build-windows       # Windows

# API 서버
make build-api           # 현재 OS용
make build-api-linux     # Linux (서버 배포용)
```

**요구사항**: Go 1.21+, macOS 12+ / Windows 10+

## API 서버

게임 데이터(강화 확률, 판매가, 역배 보상)를 제공하는 API 서버입니다.

```bash
# 로컬 실행
make run-api

# 또는
PORT=8080 go run ./cmd/sword-api
```

**엔드포인트**:
- `GET /api/game-data` - 게임 데이터 조회
- `POST /api/telemetry` - 텔레메트리 수신
- `GET /api/stats/detailed` - 커뮤니티 통계

## 기술

| | macOS | Windows |
|--|-------|---------|
| 캡처 | ScreenCaptureKit | GDI32 |
| OCR | Vision Framework | Windows.Media.Ocr |
| 입력 | CGEvent | SendInput |

## 데이터 수집 안내

이 매크로는 서비스 개선을 위해 **익명화된 사용 통계**를 수집합니다.

### 수집 항목
- 강화 시도/성공/실패/파괴 횟수
- 배틀 횟수 및 승패
- 파밍/판매 통계
- 앱 버전, OS 종류

### 수집하지 않는 항목
- 개인 식별 정보 (IP, 계정명 등)
- 카카오톡 대화 내용
- 화면 캡처 이미지
- 키보드/마우스 입력 원본

### 보안
- 모든 통신은 HTTPS로 암호화
- 세션 ID는 SHA-256 해시로 익명화
- 텔레메트리 전송 시 서명 검증으로 위변조 방지
- 수집된 데이터는 통계 목적으로만 사용

수집된 통계는 `/api/stats/detailed` 엔드포인트에서 누구나 확인 가능합니다.

## License

MIT
