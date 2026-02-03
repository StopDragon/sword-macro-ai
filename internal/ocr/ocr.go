package ocr

import "image"

// Engine OCR 엔진 인터페이스
type Engine interface {
	Init() error
	Recognize(img *image.RGBA) (string, error)
	Close()
}

// engine 플랫폼별 OCR 엔진
var engine Engine

// Init OCR 엔진 초기화
func Init() error {
	engine = newEngine()
	return engine.Init()
}

// Recognize 이미지에서 텍스트 인식
func Recognize(img *image.RGBA) (string, error) {
	if engine == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	return engine.Recognize(img)
}

// Close OCR 엔진 종료
func Close() {
	if engine != nil {
		engine.Close()
	}
}
