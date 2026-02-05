package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/StopDragon/sword-macro-ai/internal/config"
	"github.com/StopDragon/sword-macro-ai/internal/console"
	"github.com/StopDragon/sword-macro-ai/internal/game"
	"github.com/StopDragon/sword-macro-ai/internal/logger"
	"github.com/StopDragon/sword-macro-ai/internal/telemetry"
)

func init() {
	// macOS에서 Cocoa UI를 사용하려면 메인 스레드 고정 필요
	runtime.LockOSThread()
}

const VERSION = "2.8.0"

func main() {
	// Windows 콘솔 ANSI 지원 활성화 및 UTF-8 설정
	console.Init()

	fmt.Println("===========================================")
	fmt.Println("  검키우기 매크로 v" + VERSION + " (Go)")
	fmt.Println("  macOS / Windows 크로스플랫폼")
	fmt.Println("===========================================")
	fmt.Println()

	// 로거 초기화
	logger.Init()
	defer logger.Close()

	// 텔레메트리 초기화
	telem := telemetry.New(VERSION)
	defer telem.Flush()

	// 설정 로드
	cfg, err := config.Load()
	if err != nil {
		logger.Error("설정 로드 실패: %v", err)
		cfg = config.Default()
	}

	// 게임 엔진 생성
	engine := game.NewEngine(cfg, telem)

	// 시그널 핸들링 (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n프로그램을 종료합니다...")
		engine.Stop()
		os.Exit(0)
	}()

	// 메인 메뉴 실행
	engine.RunMenu()
}
