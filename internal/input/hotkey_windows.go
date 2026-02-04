//go:build windows

package input

import "fmt"

var (
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

const (
	VK_F8 = 0x77 // Windows virtual key code for F8
	VK_F9 = 0x78 // Windows virtual key code for F9
)

// f8/f9 이전 상태 (edge detection: 눌린 순간만 감지)
var (
	f8WasPressed bool
	f9WasPressed bool
)

// startPlatformHotkeys Windows 핫키 리스닝 시작
func startPlatformHotkeys() {
	// GetAsyncKeyState는 폴링 방식이라 별도 초기화 불필요
	f8WasPressed = false
	f9WasPressed = false
	fmt.Println("✅ 핫키 활성화 (F8: 일시정지, F9: 종료)")
}

// stopPlatformHotkeys Windows 핫키 리스닝 중지
func stopPlatformHotkeys() {
	f8WasPressed = false
	f9WasPressed = false
}

// isKeyDown GetAsyncKeyState로 키 눌림 확인
func isKeyDown(vk uintptr) bool {
	ret, _, _ := getAsyncKeyState.Call(vk)
	// 최상위 비트가 1이면 현재 눌려 있음
	return ret&0x8000 != 0
}

// checkF8 F8 키 눌림 확인 (edge detection: 누른 순간만 true)
func checkF8() bool {
	pressed := isKeyDown(VK_F8)
	if pressed && !f8WasPressed {
		f8WasPressed = true
		return true
	}
	if !pressed {
		f8WasPressed = false
	}
	return false
}

// checkF9 F9 키 눌림 확인 (edge detection: 누른 순간만 true)
func checkF9() bool {
	pressed := isKeyDown(VK_F9)
	if pressed && !f9WasPressed {
		f9WasPressed = true
		return true
	}
	if !pressed {
		f9WasPressed = false
	}
	return false
}
