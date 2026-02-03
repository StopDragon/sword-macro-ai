package input

import (
	"sync"
)

// HotkeyCallback 핫키 콜백 함수
type HotkeyCallback func()

// HotkeyManager 핫키 관리자
// 참고: 핫키 기능은 플랫폼별 구현이 필요하며, 현재는 기본 구조만 제공
type HotkeyManager struct {
	callbacks map[uint16]HotkeyCallback
	running   bool
	mu        sync.Mutex
}

// 키 코드 상수
const (
	KeyF8  uint16 = 119 // F8
	KeyF9  uint16 = 120 // F9
	KeyF10 uint16 = 121 // F10
)

// NewHotkeyManager 핫키 관리자 생성
func NewHotkeyManager() *HotkeyManager {
	return &HotkeyManager{
		callbacks: make(map[uint16]HotkeyCallback),
	}
}

// Register 핫키 등록
func (h *HotkeyManager) Register(keyCode uint16, callback HotkeyCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[keyCode] = callback
}

// Start 핫키 리스닝 시작
// 참고: 현재 버전에서는 핫키 감지 미구현 (터미널 기반으로 동작)
func (h *HotkeyManager) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = true
	// 핫키 리스닝은 추후 구현
	// 현재는 CheckFailsafe()를 통한 비상정지만 지원
}

// Stop 핫키 리스닝 중지
func (h *HotkeyManager) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = false
}
