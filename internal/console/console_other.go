//go:build !windows

package console

// Init is a no-op on non-Windows platforms (macOS/Linux already support ANSI)
func Init() error {
	return nil
}

// KeepOpen is a no-op on non-Windows platforms
func KeepOpen() {
	// Not needed on macOS/Linux
}
