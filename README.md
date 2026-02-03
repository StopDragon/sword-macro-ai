# sword-macro-ai

카카오톡 **검키우기** 게임 자동화 매크로 (macOS / Windows)

> **Go로 재작성** — 바이너리 크기 500MB → 2.3MB, 외부 의존성 제로

---

## 다운로드

### ⬇️ [최신 버전 다운로드](../../releases/latest)

| 플랫폼 | 파일 | 크기 | 설치 방법 |
|--------|------|------|-----------|
| **macOS** | `SwordMacro-macOS.zip` | ~5MB | 압축 해제 → 실행 |
| **Windows** | `SwordMacro-Windows.zip` | ~3MB | 압축 해제 → `SwordMacro.exe` 실행 |

<details>
<summary>📋 상세 설치 가이드 (클릭)</summary>

#### macOS
1. ZIP 압축 해제 → `SwordMacro` 파일 생성
2. 첫 실행 시 **"확인되지 않은 개발자"** 경고 → 우클릭 → **열기**
3. **접근성 권한 허용** (필수): 시스템 설정 > 개인정보 보호 및 보안 > 접근성 > `SwordMacro` 체크
4. **화면 기록 권한 허용** (필수): 시스템 설정 > 개인정보 보호 및 보안 > 화면 기록 > `SwordMacro` 체크

#### Windows
1. ZIP 압축 해제 → `SwordMacro.exe` 생성
2. 실행
3. Windows Defender 경고 시: **추가 정보** → **실행**
4. 한국어 OCR 언어팩 필요 (Windows 설정 > 시간 및 언어 > 언어에서 한국어 추가)

</details>

---

## 주요 기능

| 모드 | 설명 |
|------|------|
| **강화 목표 달성** | 설정한 강화 레벨까지 자동 강화 |
| **히든 검 뽑기** | 히든 아이템 발견까지 자동 파밍 |
| **골드 채굴** | 파밍 → 강화(+10) → 판매 무한 순환 |

### 부가 기능
- **핫키** — F8 일시정지/재개, F9 재시작
- **비상 정지** — 마우스를 화면 좌상단 모서리로 이동

---

## 기술 스택

| 기능 | macOS | Windows |
|------|-------|---------|
| **화면 캡처** | ScreenCaptureKit | GDI32 BitBlt |
| **OCR** | Vision Framework | Windows.Media.Ocr |
| **입력 자동화** | CGEvent API | SendInput API |

- **Go 1.21** — 외부 의존성 없음, 네이티브 API만 사용
- **바이너리 크기**: ~2.3MB (macOS), ~3MB (Windows)

---

## 요구 사항

- **macOS** 12+ (ScreenCaptureKit 지원)
- **Windows** 10+ (한국어 OCR 언어팩 필요)
- 카카오톡 데스크톱 앱

---

## 사용법

### 1. 카카오톡 준비
- 카카오톡에서 검키우기 채팅방을 엽니다
- 채팅창 크기를 적당히 조절합니다

### 2. 매크로 실행
다운로드한 파일을 실행합니다.

### 3. 모드 선택
```
=== 카카오톡 검키우기 ===
1. 강화 목표 달성
2. 히든 검 뽑기
3. 골드 채굴 (돈벌기)
4. 옵션 설정
```

### 4. 좌표 설정
- 카카오톡 **메시지 입력 칸**에 마우스를 올리세요
- 3초 후 자동으로 좌표를 저장합니다

### 5. 조작 키
| 키 | 동작 |
|----|------|
| `F8` | 일시정지 / 재개 |
| `F9` | 재시작 (메뉴로 복귀) |
| 마우스 좌상단 | 비상 정지 |

---

## 옵션 설정

| 항목 | 기본값 | 설명 |
|------|--------|------|
| 감속 시작 레벨 | +9 | 이 레벨부터 강화 딜레이 증가 |
| 중간 속도 | 2.5초 | 중간 레벨(+5~+8) 강화 대기 |
| 고강 속도 | 3.5초 | 고레벨(+9~) 강화 대기 |
| 좌표 고정 | OFF | 매번 좌표 설정 없이 저장된 위치 사용 |
| 골드 채굴 목표 | +10 | 골드 채굴 모드 목표 레벨 |

설정은 `sword_config.json`에 자동 저장됩니다.

---

## 직접 빌드하기 (개발자용)

### 요구 사항
- Go 1.21+
- macOS: Xcode Command Line Tools
- Windows: MinGW-w64 (크로스 컴파일 시)

### 빌드

```bash
# 저장소 클론
git clone https://github.com/StopDragon/sword-macro-ai.git
cd sword-macro-ai

# macOS 빌드
make build-mac

# macOS Universal Binary (Intel + Apple Silicon)
make build-mac-universal

# Windows 빌드 (Windows에서)
make build-windows
```

### 생성 파일

| 파일 | 위치 |
|------|------|
| macOS 바이너리 | `build/SwordMacro` |
| Windows 바이너리 | `build/SwordMacro.exe` |

---

## 주의사항

- 매크로 실행 중 카카오톡 창을 이동하거나 가리지 마세요
- OCR은 화면 캡처 기반이므로 다른 창이 겹치면 인식이 실패합니다
- macOS: 접근성 + 화면 기록 권한 모두 필요
- Windows: 한국어 OCR 언어팩 필요

---

## 생성 파일

| 파일 | 용도 |
|------|------|
| `sword_config.json` | 설정 저장 |
| `sword_macro.log` | 런타임 로그 |

---

## License

MIT
