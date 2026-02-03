package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LogFile    = "sword_macro.log"
	MaxLogSize = 5 * 1024 * 1024 // 5MB
)

var (
	file           *os.File
	logger         *log.Logger
	lastLoggedText string // 마지막 로깅된 텍스트 (중복 방지)
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

// ChatText 채팅 텍스트 로그 (새로운 부분만 기록)
func ChatText(text string) {
	if logger == nil || text == "" {
		return
	}

	// 텍스트 정규화 (trailing whitespace 제거)
	normalizedText := strings.TrimSpace(text)
	if normalizedText == "" {
		return
	}

	// 이전과 동일하면 스킵
	if normalizedText == lastLoggedText {
		return
	}

	// 새로운 줄만 추출 (이전 텍스트에 없던 줄)
	newLines := extractNewLines(lastLoggedText, normalizedText)
	lastLoggedText = normalizedText

	if newLines == "" {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logger.Printf("[%s] CHAT:\n%s\n---", timestamp, newLines)
}

// extractNewLines 이전 텍스트와 비교하여 새로운 줄만 추출
// Old: ABCDE, New: ABCDEABFG → 반환: ABFG
func extractNewLines(oldText, newText string) string {
	if oldText == "" {
		return newText
	}

	if oldText == newText {
		return ""
	}

	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	// 방법 1: oldLines가 newLines의 앞부분과 일치하는지 확인 (채팅 추가 케이스)
	matchCount := 0
	for i := 0; i < len(oldLines) && i < len(newLines); i++ {
		if strings.TrimSpace(oldLines[i]) == strings.TrimSpace(newLines[i]) {
			matchCount++
		} else {
			break
		}
	}

	// 전체 또는 대부분 일치하면 나머지 반환
	if matchCount == len(oldLines) && matchCount < len(newLines) {
		return strings.Join(newLines[matchCount:], "\n")
	}

	// 방법 2: 채팅이 스크롤되어 oldLines의 뒷부분만 newLines 앞에 남은 경우
	// oldLines의 suffix가 newLines의 prefix와 일치하는지 확인
	for suffixStart := 1; suffixStart < len(oldLines); suffixStart++ {
		suffix := oldLines[suffixStart:]
		if len(suffix) <= len(newLines) && linesMatch(suffix, newLines[:len(suffix)]) {
			// suffix 이후의 새 내용 반환
			if len(suffix) < len(newLines) {
				return strings.Join(newLines[len(suffix):], "\n")
			}
			return ""
		}
	}

	// 일치하는 부분 없음 - 전체가 새 내용
	return newText
}

// linesMatch 두 줄 배열이 동일한지 비교
func linesMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

// ResetChatLog 채팅 로그 상태 초기화 (세션 시작 시)
func ResetChatLog() {
	lastLoggedText = ""
}

func getLogPath() string {
	exe, err := os.Executable()
	if err != nil {
		return LogFile
	}
	return filepath.Join(filepath.Dir(exe), LogFile)
}
