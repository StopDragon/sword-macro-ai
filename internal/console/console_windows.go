//go:build windows

package console

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle               = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procSetConsoleOutputCP         = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleTitleW           = kernel32.NewProc("SetConsoleTitleW")
)

const (
	STD_OUTPUT_HANDLE              = ^uintptr(0) - 10 + 1 // -11
	STD_ERROR_HANDLE               = ^uintptr(0) - 10     // -12
	ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
	CP_UTF8                        = 65001
)

// Init initializes the Windows console for ANSI escape code support and UTF-8
func Init() error {
	// Set console output code page to UTF-8
	procSetConsoleOutputCP.Call(CP_UTF8)

	// Get stdout handle
	handle, _, _ := procGetStdHandle.Call(STD_OUTPUT_HANDLE)
	if handle == 0 || handle == ^uintptr(0) {
		return nil // No console attached
	}

	// Get current console mode
	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return nil // Failed to get mode
	}

	// Enable Virtual Terminal Processing for ANSI escape codes
	mode |= ENABLE_VIRTUAL_TERMINAL_PROCESSING
	ret, _, _ = procSetConsoleMode.Call(handle, uintptr(mode))
	if ret == 0 {
		// If enabling VT processing fails, that's okay on older Windows
		// The program will still work, just without ANSI colors
		return nil
	}

	// Also enable for stderr
	handleErr, _, _ := procGetStdHandle.Call(STD_ERROR_HANDLE)
	if handleErr != 0 && handleErr != ^uintptr(0) {
		var modeErr uint32
		ret, _, _ = procGetConsoleMode.Call(handleErr, uintptr(unsafe.Pointer(&modeErr)))
		if ret != 0 {
			modeErr |= ENABLE_VIRTUAL_TERMINAL_PROCESSING
			procSetConsoleMode.Call(handleErr, uintptr(modeErr))
		}
	}

	// Set console title
	title, _ := syscall.UTF16PtrFromString("검키우기 매크로")
	procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(title)))

	return nil
}

// KeepOpen prevents the console from closing immediately on exit
// Call this before os.Exit if you want to wait for user input
func KeepOpen() {
	fmt.Println("\n프로그램을 종료하려면 Enter 키를 누르세요...")
	os.Stdin.Read(make([]byte, 1))
}
