package input

import (
	"sync"
)

// HotkeyCallback 핫키 콜백 함수
type HotkeyCallback func()

// HotkeyManager 핫키 관리자
type HotkeyManager struct {
	callbacks map[uint16]HotkeyCallback
	running   bool
	mu        sync.Mutex
}

// 키 코드 상수 (macOS virtual keycodes)
const (
	KeyF8  uint16 = 100 // kVK_F8 = 0x64
	KeyF9  uint16 = 101 // kVK_F9 = 0x65
	KeyF10 uint16 = 109 // kVK_F10 = 0x6D
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

// Start 핫키 리스닝 시작 (플랫폼별 CGEventTap / RegisterHotKey)
func (h *HotkeyManager) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = true
	startPlatformHotkeys()
}

// Stop 핫키 리스닝 중지
func (h *HotkeyManager) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = false
	stopPlatformHotkeys()
}

// CheckF8Pressed F8 키 눌림 확인 (확인 후 플래그 리셋)
func CheckF8Pressed() bool {
	return checkF8()
}

// CheckF9Pressed F9 키 눌림 확인 (확인 후 플래그 리셋)
func CheckF9Pressed() bool {
	return checkF9()
}
