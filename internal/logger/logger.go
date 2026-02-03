package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	LogFile    = "sword_macro.log"
	MaxLogSize = 5 * 1024 * 1024 // 5MB
)

var (
	file   *os.File
	logger *log.Logger
)

// Init 로거 초기화
func Init() {
	logPath := getLogPath()

	// 로그 파일 크기 확인 및 로테이션
	if info, err := os.Stat(logPath); err == nil {
		if info.Size() > MaxLogSize {
			os.Rename(logPath, logPath+".bak")
		}
	}

	var err error
	file, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("로그 파일 열기 실패: %v", err)
		return
	}

	logger = log.New(file, "", 0)
	Info("=== 세션 시작 ===")
}

// Close 로거 종료
func Close() {
	if file != nil {
		Info("=== 세션 종료 ===")
		file.Close()
	}
}

// Info 정보 로그
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if logger != nil {
		logger.Printf("[%s] INFO: %s", timestamp, msg)
	}
}

// Error 에러 로그
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if logger != nil {
		logger.Printf("[%s] ERROR: %s", timestamp, msg)
	}
	fmt.Printf("[오류] %s\n", msg)
}

// Debug 디버그 로그
func Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if logger != nil {
		logger.Printf("[%s] DEBUG: %s", timestamp, msg)
	}
}

// OCR OCR 결과 로그
func OCR(text string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if logger != nil {
		logger.Printf("[%s] OCR:\n%s\n---", timestamp, text)
	}
}

func getLogPath() string {
	exe, err := os.Executable()
	if err != nil {
		return LogFile
	}
	return filepath.Join(filepath.Dir(exe), LogFile)
}
