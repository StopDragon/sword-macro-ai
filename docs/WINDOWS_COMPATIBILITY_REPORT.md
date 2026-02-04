# Windows 호환성 검토 보고서

**작성일**: 2026-02-03
**검토 대상**: 상태 패널 판단 메시지 추가 및 프로필 조회 개선

---

## 검토 대상 파일

| 파일 | 변경 내용 |
|------|----------|
| `internal/game/engine.go` | 상태 패널 판단 메시지 추가, showMyProfile() "0" 입력 대기 |
| `internal/overlay/overlay_windows.go` | Windows 오버레이 구현 (기존) |

---

## 발견된 문제점

### 1. TextOutW 멀티라인 미지원 (심각도: 높음)

| 항목 | macOS | Windows |
|------|-------|---------|
| 텍스트 렌더링 | NSTextField (멀티라인 자동 지원) | TextOutW (단일 라인만) |
| `\n` 처리 | 자동 줄바꿈 | 무시됨 |

**문제 설명**:
새로 추가된 메시지들이 `\n`을 포함하고 있습니다:

```go
overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n🎉 특수 발견!\n[%s]\n\n📋 판단: 특수 → 보관/강화", itemName)
```

Windows의 `TextOutW` API는 개행 문자를 처리하지 않아 **모든 텍스트가 한 줄로 표시**됩니다.

**영향 받는 코드**:
- `loopSpecial()` - 특수/쓰레기 감지 메시지
- `loopGoldMine()` - 파밍/강화/판매 메시지
- `loopBattle()` - 타겟 선택/결과 메시지
- `enhanceToTargetWithLevel()` - 강화 결과 메시지
- `showMyProfile()` - 프로필 요약 메시지

---

### 2. 텍스트 길이 계산 오류 (심각도: 높음)

**위치**: `internal/overlay/overlay_windows.go:180`

```go
procTextOutW.Call(hdc, 10, 10, uintptr(unsafe.Pointer(textPtr)), uintptr(len(statusText)))
```

| 문제 | 설명 |
|------|------|
| `len(statusText)` | Go 바이트 길이 반환 |
| TextOutW 기대값 | UTF-16 **문자 수** |

**예시**:
| 텍스트 | `len()` 결과 | 실제 문자 수 |
|--------|-------------|-------------|
| `"📋 판단"` | 13 bytes | 5 characters |
| `"한글"` | 6 bytes | 2 characters |
| `"ABC"` | 3 bytes | 3 characters |

한글과 이모지가 포함된 텍스트에서 **텍스트가 잘리거나 깨짐** 현상이 발생합니다.

---

### 3. 상태 패널 크기 vs 텍스트 양 불일치 (심각도: 중간)

| 항목 | 값 |
|------|-----|
| 상태 패널 너비 | 280px |
| 상태 패널 높이 | 430px |
| 텍스트 시작 위치 | (10, 10) |
| 폰트 크기 | 14px (Consolas) |

새로운 메시지 예시 (5줄):
```
⭐ 특수 아이템 뽑기
쓰레기: 15회
🗑️ 고대의 검

📋 판단: 쓰레기 → 파괴
```

Windows에서 한 줄로 압축되어 **280px 내에서 대부분 잘림** 현상이 발생합니다.

---

## 정상 작동 부분

| 기능 | 상태 | 비고 |
|------|------|------|
| `UpdateStatus()` 호출 | ✅ 정상 | 함수 시그니처 동일 |
| `InvalidateRect` + `UpdateWindow` | ✅ 정상 | 화면 갱신 정상 동작 |
| `showMyProfile()` 입력 대기 | ✅ 정상 | `bufio.Reader`는 플랫폼 독립적 |
| 컨트롤 패널 버튼 | ❌ 미구현 | F8/F9 핫키로 대체 (기존 사양) |

---

## 영향도 분석

| 심각도 | 문제 | 영향 범위 |
|--------|------|----------|
| 🔴 높음 | 멀티라인 미지원 | 모든 UpdateStatus 호출 (12개소) |
| 🔴 높음 | 텍스트 길이 오류 | 한글/이모지 포함 메시지 전체 |
| 🟡 중간 | 텍스트 오버플로우 | 긴 메시지 표시 시 |
| 🟢 낮음 | 컨트롤 패널 미구현 | F8/F9 핫키로 대체 가능 |

---

## 권장 수정 사항

### 방안 1: DrawTextW API로 교체 (권장)

`statusWndProc` 함수에서 `TextOutW`를 `DrawTextW`로 교체:

```go
// 기존
procTextOutW.Call(hdc, 10, 10, uintptr(unsafe.Pointer(textPtr)), uintptr(len(statusText)))

// 개선
const DT_WORDBREAK = 0x0010
const DT_TOP = 0x0000
const DT_LEFT = 0x0000

rect := RECT{10, 10, int32(statusW - 10), int32(statusH - 10)}
procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(textPtr)), uintptr(0xFFFFFFFF),
    uintptr(unsafe.Pointer(&rect)), DT_WORDBREAK|DT_TOP|DT_LEFT)
```

**장점**:
- 멀티라인 자동 지원 (`\n` 처리)
- 자동 줄바꿈 (DT_WORDBREAK)
- 텍스트 길이 -1 전달 시 null-terminated 문자열로 처리

### 방안 2: UTF-16 문자 수 계산 (임시 해결책)

```go
import "unicode/utf16"

// 기존
uintptr(len(statusText))

// 개선
utf16Chars := utf16.Encode([]rune(statusText))
uintptr(len(utf16Chars))
```

---

## 플랫폼별 테스트 결과 예상

| 플랫폼 | 판단 메시지 표시 | 프로필 "0" 대기 | 전체 평가 |
|--------|-----------------|----------------|----------|
| macOS | ✅ 정상 (멀티라인) | ✅ 정상 | ✅ 완전 호환 |
| Windows | ⚠️ 한 줄 + 잘림 | ✅ 정상 | ⚠️ 부분 호환 |

---

## 결론

`engine.go`의 변경사항은 로직적으로 올바르나, Windows 오버레이의 텍스트 렌더링 방식(`TextOutW`)이 멀티라인과 유니코드를 제대로 처리하지 못해 **Windows에서 상태 패널 표시가 비정상적**입니다.

**우선순위**: Windows 사용자가 많다면 `overlay_windows.go`의 `DrawTextW` 마이그레이션을 권장합니다.

---

## 참고 자료

- [TextOutW (Microsoft Docs)](https://docs.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-textoutw)
- [DrawTextW (Microsoft Docs)](https://docs.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-drawtextw)
- [Go UTF-16 Package](https://pkg.go.dev/unicode/utf16)
