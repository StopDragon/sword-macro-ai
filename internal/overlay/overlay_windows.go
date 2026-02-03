// +build windows

package overlay

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	gdi32                          = syscall.NewLazyDLL("gdi32.dll")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procUpdateWindow               = user32.NewProc("UpdateWindow")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procBeginPaint                 = user32.NewProc("BeginPaint")
	procEndPaint                   = user32.NewProc("EndPaint")
	procCreatePen                  = gdi32.NewProc("CreatePen")
	procSelectObject               = gdi32.NewProc("SelectObject")
	procDeleteObject               = gdi32.NewProc("DeleteObject")
	procRectangle                  = gdi32.NewProc("Rectangle")
	procGetStockObject             = gdi32.NewProc("GetStockObject")
	procSetBkMode                  = gdi32.NewProc("SetBkMode")
	procSetTextColor               = gdi32.NewProc("SetTextColor")
	procTextOutW                   = gdi32.NewProc("TextOutW")
	procCreateFontW                = gdi32.NewProc("CreateFontW")
	procFillRect                   = user32.NewProc("FillRect")
	procCreateSolidBrush           = gdi32.NewProc("CreateSolidBrush")
	procInvalidateRect             = user32.NewProc("InvalidateRect")
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
	HWND_TOPMOST      = ^uintptr(0)
	SWP_NOMOVE        = 0x0002
	SWP_NOSIZE        = 0x0001
	SWP_SHOWWINDOW    = 0x0040
	PS_SOLID          = 0
	NULL_BRUSH        = 5
	TRANSPARENT       = 1
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
	ocrHwnd           uintptr
	inputHwnd         uintptr
	statusHwnd        uintptr
	ocrW, ocrH        int
	inputW, inputH    int
	statusW, statusH  int
	statusText        string = "🎮 대기 중..."
	classRegistered   bool
	ocrClassReg       bool
	inputClassReg     bool
	statusClassReg    bool
)

func utf16PtrFromString(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// OCR 윈도우 프로시저 (빨간색)
func ocrWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	const WM_PAINT = 0x000F

	if msg == WM_PAINT {
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		pen, _, _ := procCreatePen.Call(PS_SOLID, 2, 0x0000FF) // Red (BGR)
		oldPen, _, _ := procSelectObject.Call(hdc, pen)
		nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
		oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
		procRectangle.Call(hdc, 0, 0, uintptr(ocrW), uintptr(ocrH))
		procSelectObject.Call(hdc, oldPen)
		procSelectObject.Call(hdc, oldBrush)
		procDeleteObject.Call(pen)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// 입력창 윈도우 프로시저 (초록색)
func inputWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	const WM_PAINT = 0x000F

	if msg == WM_PAINT {
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		pen, _, _ := procCreatePen.Call(PS_SOLID, 2, 0x00FF00) // Green (BGR)
		oldPen, _, _ := procSelectObject.Call(hdc, pen)
		nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
		oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
		procRectangle.Call(hdc, 0, 0, uintptr(inputW), uintptr(inputH))
		procSelectObject.Call(hdc, oldPen)
		procSelectObject.Call(hdc, oldBrush)
		procDeleteObject.Call(pen)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// 상태 윈도우 프로시저 (검은 배경 + 흰 텍스트)
func statusWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	const WM_PAINT = 0x000F

	if msg == WM_PAINT {
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

		// 검은 배경
		brush, _, _ := procCreateSolidBrush.Call(0x202020) // Dark gray
		rect := RECT{0, 0, int32(statusW), int32(statusH)}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
		procDeleteObject.Call(brush)

		// 폰트 생성
		font, _, _ := procCreateFontW.Call(
			14, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0,
			uintptr(unsafe.Pointer(utf16PtrFromString("Consolas"))),
		)
		oldFont, _, _ := procSelectObject.Call(hdc, font)

		// 텍스트 설정
		procSetBkMode.Call(hdc, TRANSPARENT)
		procSetTextColor.Call(hdc, 0xFFFFFF) // White

		// 텍스트 출력
		textPtr, _ := syscall.UTF16PtrFromString(statusText)
		procTextOutW.Call(hdc, 10, 10, uintptr(unsafe.Pointer(textPtr)), uintptr(len(statusText)))

		procSelectObject.Call(hdc, oldFont)
		procDeleteObject.Call(font)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func registerClass(className string, wndProc func(uintptr, uint32, uintptr, uintptr) uintptr) {
	classNamePtr := utf16PtrFromString(className)
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.LpszClassName = classNamePtr
	wc.HbrBackground = 0
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

// Init 오버레이 시스템 초기화
func Init() {
	if !ocrClassReg {
		registerClass("SwordOCROverlay", ocrWndProc)
		ocrClassReg = true
	}
	if !inputClassReg {
		registerClass("SwordInputOverlay", inputWndProc)
		inputClassReg = true
	}
	if !statusClassReg {
		registerClass("SwordStatusOverlay", statusWndProc)
		statusClassReg = true
	}
}

func createOverlayWindow(className string, x, y, w, h int, alpha byte) uintptr {
	classNamePtr := utf16PtrFromString(className)
	hwnd, _, _ := procCreateWindowExW.Call(
		WS_EX_LAYERED|WS_EX_TRANSPARENT|WS_EX_TOPMOST|WS_EX_TOOLWINDOW,
		uintptr(unsafe.Pointer(classNamePtr)),
		0,
		WS_POPUP|WS_VISIBLE,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		0, 0, 0, 0,
	)
	if hwnd != 0 {
		procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), LWA_ALPHA)
		procSetWindowPos.Call(hwnd, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
		procShowWindow.Call(hwnd, SW_SHOW)
		procUpdateWindow.Call(hwnd)
	}
	return hwnd
}

// Show OCR 캡처 영역 오버레이 표시 (하위 호환)
func Show(x, y, width, height int) {
	ShowOCRRegion(x, y, width, height)
}

// Hide 오버레이 숨기기 (하위 호환)
func Hide() {
	HideAll()
}

// ShowOCRRegion OCR 영역 표시 (빨간색)
func ShowOCRRegion(x, y, width, height int) {
	Init()
	if ocrHwnd != 0 {
		procDestroyWindow.Call(ocrHwnd)
	}
	ocrW, ocrH = width, height
	ocrHwnd = createOverlayWindow("SwordOCROverlay", x, y, width, height, 200)
}

// ShowInputRegion 입력창 영역 표시 (초록색)
func ShowInputRegion(x, y, width, height int) {
	Init()
	if inputHwnd != 0 {
		procDestroyWindow.Call(inputHwnd)
	}
	inputW, inputH = width, height
	inputHwnd = createOverlayWindow("SwordInputOverlay", x, y, width, height, 200)
}

// ShowStatusPanel 상태 패널 표시
func ShowStatusPanel(x, y, width, height int) {
	Init()
	if statusHwnd != 0 {
		procDestroyWindow.Call(statusHwnd)
	}
	statusW, statusH = width, height
	statusHwnd = createOverlayWindow("SwordStatusOverlay", x, y, width, height, 230)
}

// ShowAll 모든 오버레이 표시
func ShowAll(ocrX, ocrY, ocrW, ocrH, inputX, inputY, inputW, inputH int) {
	Init()
	ShowOCRRegion(ocrX, ocrY, ocrW, ocrH)
	ShowInputRegion(inputX, inputY, inputW, inputH)

	statusX := ocrX + ocrW + 10
	statusY := ocrY
	ShowStatusPanel(statusX, statusY, 280, 150)
}

// UpdateStatus 상태 텍스트 업데이트
func UpdateStatus(format string, args ...interface{}) {
	statusText = fmt.Sprintf(format, args...)
	if statusHwnd != 0 {
		procInvalidateRect.Call(statusHwnd, 0, 1)
		procUpdateWindow.Call(statusHwnd)
	}
}

// HideAll 모든 오버레이 숨기기
func HideAll() {
	if ocrHwnd != 0 {
		procDestroyWindow.Call(ocrHwnd)
		ocrHwnd = 0
	}
	if inputHwnd != 0 {
		procDestroyWindow.Call(inputHwnd)
		inputHwnd = 0
	}
	if statusHwnd != 0 {
		procDestroyWindow.Call(statusHwnd)
		statusHwnd = 0
	}
}

// ShowForDuration 지정 시간 동안 오버레이 표시
func ShowForDuration(x, y, width, height int, duration time.Duration) {
	Show(x, y, width, height)
	time.Sleep(duration)
	Hide()
}
