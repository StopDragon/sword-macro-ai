// +build windows

package overlay

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	gdi32                   = syscall.NewLazyDLL("gdi32.dll")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procInvalidateRect      = user32.NewProc("InvalidateRect")
	procBeginPaint          = user32.NewProc("BeginPaint")
	procEndPaint            = user32.NewProc("EndPaint")
	procCreatePen           = gdi32.NewProc("CreatePen")
	procCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procRectangle           = gdi32.NewProc("Rectangle")
	procGetStockObject      = gdi32.NewProc("GetStockObject")
)

const (
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_POPUP          = 0x80000000
	WS_VISIBLE        = 0x10000000
	SW_SHOW           = 5
	SW_HIDE           = 0
	LWA_COLORKEY      = 0x00000001
	LWA_ALPHA         = 0x00000002
	HWND_TOPMOST      = ^uintptr(0) // -1
	SWP_NOMOVE        = 0x0002
	SWP_NOSIZE        = 0x0001
	SWP_SHOWWINDOW    = 0x0040
	PS_SOLID          = 0
	NULL_BRUSH        = 5
	SM_CXSCREEN       = 0
	SM_CYSCREEN       = 1
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type PAINTSTRUCT struct {
	HDC         syscall.Handle
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

var (
	hwnd          uintptr
	overlayX      int
	overlayY      int
	overlayWidth  int
	overlayHeight int
	classRegistered bool
)

func utf16PtrFromString(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	const (
		WM_PAINT   = 0x000F
		WM_DESTROY = 0x0002
	)

	switch msg {
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

		// 빨간색 펜 (3픽셀 두께)
		pen, _, _ := procCreatePen.Call(PS_SOLID, 3, 0x0000FF) // BGR: Red
		oldPen, _, _ := procSelectObject.Call(hdc, pen)

		// 투명 브러시
		nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
		oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)

		// 사각형 그리기
		procRectangle.Call(hdc, 0, 0, uintptr(overlayWidth), uintptr(overlayHeight))

		// 복원
		procSelectObject.Call(hdc, oldPen)
		procSelectObject.Call(hdc, oldBrush)
		procDeleteObject.Call(pen)

		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// Init 오버레이 시스템 초기화
func Init() {
	if classRegistered {
		return
	}

	className := utf16PtrFromString("SwordOverlayClass")

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.LpszClassName = className
	wc.HbrBackground = 0

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	classRegistered = true
}

// Show OCR 캡처 영역 오버레이 표시
func Show(x, y, width, height int) {
	if !classRegistered {
		Init()
	}

	// 기존 윈도우 제거
	if hwnd != 0 {
		Hide()
	}

	overlayX = x
	overlayY = y
	overlayWidth = width
	overlayHeight = height

	className := utf16PtrFromString("SwordOverlayClass")

	// 레이어드 윈도우 생성
	hwnd, _, _ = procCreateWindowExW.Call(
		WS_EX_LAYERED|WS_EX_TRANSPARENT|WS_EX_TOPMOST|WS_EX_TOOLWINDOW,
		uintptr(unsafe.Pointer(className)),
		0,
		WS_POPUP|WS_VISIBLE,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, 0, 0,
	)

	if hwnd != 0 {
		// 투명도 설정 (마젠타를 투명색으로)
		procSetLayeredWindowAttributes.Call(hwnd, 0x00FF00FF, 200, LWA_COLORKEY|LWA_ALPHA)

		// 최상위에 표시
		procSetWindowPos.Call(hwnd, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
		procShowWindow.Call(hwnd, SW_SHOW)
		procUpdateWindow.Call(hwnd)
	}
}

// Hide 오버레이 숨기기
func Hide() {
	if hwnd != 0 {
		procDestroyWindow.Call(hwnd)
		hwnd = 0
	}
}

// ShowForDuration 지정 시간 동안 오버레이 표시
func ShowForDuration(x, y, width, height int, duration time.Duration) {
	Show(x, y, width, height)
	time.Sleep(duration)
	Hide()
}
