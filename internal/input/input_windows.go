//go:build windows

package input

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getCursorPos     = user32.NewProc("GetCursorPos")
	setCursorPos     = user32.NewProc("SetCursorPos")
	sendInput        = user32.NewProc("SendInput")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	getClipboardData = user32.NewProc("GetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalSize       = kernel32.NewProc("GlobalSize")
)

const (
	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_ABSOLUTE   = 0x8000

	KEYEVENTF_KEYUP = 0x0002

	VK_RETURN  = 0x0D
	VK_CONTROL = 0x11
	VK_V       = 0x56
	VK_A       = 0x41
	VK_C       = 0x43
	VK_BACK    = 0x08 // Backspace

	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002
)

type POINT struct {
	X, Y int32
}

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type INPUT struct {
	Type uint32
	Mi   MOUSEINPUT
}

type INPUT_KB struct {
	Type uint32
	Ki   KEYBDINPUT
	_    [8]byte // padding
}

func move(x, y int) {
	setCursorPos.Call(uintptr(x), uintptr(y))
}

func click(x, y int) {
	// Move
	setCursorPos.Call(uintptr(x), uintptr(y))
	time.Sleep(10 * time.Millisecond)

	// Mouse down
	var inputDown INPUT
	inputDown.Type = INPUT_MOUSE
	inputDown.Mi.DwFlags = MOUSEEVENTF_LEFTDOWN
	sendInput.Call(1, uintptr(unsafe.Pointer(&inputDown)), unsafe.Sizeof(inputDown))

	time.Sleep(10 * time.Millisecond)

	// Mouse up
	var inputUp INPUT
	inputUp.Type = INPUT_MOUSE
	inputUp.Mi.DwFlags = MOUSEEVENTF_LEFTUP
	sendInput.Call(1, uintptr(unsafe.Pointer(&inputUp)), unsafe.Sizeof(inputUp))
}

func getMousePos() (int, int) {
	var pt POINT
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

func typeText(text string) {
	// 클립보드에 복사
	setClipboardText(text)
	time.Sleep(50 * time.Millisecond)

	// Ctrl+V
	pressKeyWithModifier(VK_V, VK_CONTROL)
}

func pressEnter() {
	pressKey(VK_RETURN)
}

func clearInput() {
	// Ctrl+A (전체 선택)
	pressKeyWithModifier(VK_A, VK_CONTROL)
	time.Sleep(50 * time.Millisecond)
	// Backspace (삭제)
	pressKey(VK_BACK)
}

func pressKey(vk uint16) {
	var inputDown INPUT_KB
	inputDown.Type = INPUT_KEYBOARD
	inputDown.Ki.WVk = vk
	sendInput.Call(1, uintptr(unsafe.Pointer(&inputDown)), unsafe.Sizeof(inputDown))

	var inputUp INPUT_KB
	inputUp.Type = INPUT_KEYBOARD
	inputUp.Ki.WVk = vk
	inputUp.Ki.DwFlags = KEYEVENTF_KEYUP
	sendInput.Call(1, uintptr(unsafe.Pointer(&inputUp)), unsafe.Sizeof(inputUp))
}

func pressKeyWithModifier(vk, modifier uint16) {
	// Modifier down
	var modDown INPUT_KB
	modDown.Type = INPUT_KEYBOARD
	modDown.Ki.WVk = modifier
	sendInput.Call(1, uintptr(unsafe.Pointer(&modDown)), unsafe.Sizeof(modDown))

	time.Sleep(10 * time.Millisecond)

	// Key down
	var keyDown INPUT_KB
	keyDown.Type = INPUT_KEYBOARD
	keyDown.Ki.WVk = vk
	sendInput.Call(1, uintptr(unsafe.Pointer(&keyDown)), unsafe.Sizeof(keyDown))

	// Key up
	var keyUp INPUT_KB
	keyUp.Type = INPUT_KEYBOARD
	keyUp.Ki.WVk = vk
	keyUp.Ki.DwFlags = KEYEVENTF_KEYUP
	sendInput.Call(1, uintptr(unsafe.Pointer(&keyUp)), unsafe.Sizeof(keyUp))

	time.Sleep(10 * time.Millisecond)

	// Modifier up
	var modUp INPUT_KB
	modUp.Type = INPUT_KEYBOARD
	modUp.Ki.WVk = modifier
	modUp.Ki.DwFlags = KEYEVENTF_KEYUP
	sendInput.Call(1, uintptr(unsafe.Pointer(&modUp)), unsafe.Sizeof(modUp))
}

func setClipboardText(text string) {
	openClipboard.Call(0)
	defer closeClipboard.Call()

	emptyClipboard.Call()

	// UTF-16 변환
	utf16 := syscall.StringToUTF16(text)
	size := len(utf16) * 2

	hMem, _, _ := globalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if hMem == 0 {
		return
	}

	pMem, _, _ := globalLock.Call(hMem)
	if pMem == 0 {
		return
	}

	// 복사
	for i, c := range utf16 {
		*(*uint16)(unsafe.Pointer(pMem + uintptr(i*2))) = c
	}

	globalUnlock.Call(hMem)
	setClipboardData.Call(CF_UNICODETEXT, hMem)
}

func selectAll() {
	// Ctrl+A (전체 선택)
	pressKeyWithModifier(VK_A, VK_CONTROL)
}

func copySelection() {
	// Ctrl+C (복사)
	pressKeyWithModifier(VK_C, VK_CONTROL)
}

func getClipboard() string {
	ret, _, _ := openClipboard.Call(0)
	if ret == 0 {
		return ""
	}
	defer closeClipboard.Call()

	hMem, _, _ := getClipboardData.Call(CF_UNICODETEXT)
	if hMem == 0 {
		return ""
	}

	pMem, _, _ := globalLock.Call(hMem)
	if pMem == 0 {
		return ""
	}
	defer globalUnlock.Call(hMem)

	// UTF-16 문자열 읽기
	size, _, _ := globalSize.Call(hMem)
	if size == 0 {
		return ""
	}

	// UTF-16을 Go string으로 변환
	utf16Slice := (*[1 << 20]uint16)(unsafe.Pointer(pMem))[:size/2:size/2]
	return syscall.UTF16ToString(utf16Slice)
}
