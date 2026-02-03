package capture

import "image"

// CaptureRegion 화면의 특정 영역을 캡처
// 플랫폼별 구현은 capture_darwin.go, capture_windows.go에서 제공
func CaptureRegion(x, y, width, height int) (*image.RGBA, error) {
	return captureRegion(x, y, width, height)
}
