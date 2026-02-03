package game

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/StopDragon/sword-macro-ai/internal/capture"
	"github.com/StopDragon/sword-macro-ai/internal/config"
	"github.com/StopDragon/sword-macro-ai/internal/input"
	"github.com/StopDragon/sword-macro-ai/internal/logger"
	"github.com/StopDragon/sword-macro-ai/internal/ocr"
)

// Mode 매크로 모드
type Mode int

const (
	ModeNone Mode = iota
	ModeEnhance      // 강화 목표 달성
	ModeHidden       // 히든 검 뽑기
	ModeGoldMine     // 골드 채굴
)

// Engine 게임 엔진
type Engine struct {
	cfg       *config.Config
	mode      Mode
	running   bool
	paused    bool
	mu        sync.Mutex

	// 상태
	currentLevel  int
	targetLevel   int
	cycleCount    int
	cycleStartTime time.Time
	totalGold     int

	// 핫키
	hotkeyMgr *input.HotkeyManager
}

// NewEngine 엔진 생성
func NewEngine(cfg *config.Config) *Engine {
	e := &Engine{
		cfg: cfg,
	}

	// 핫키 설정
	e.hotkeyMgr = input.NewHotkeyManager()
	e.hotkeyMgr.Register(input.KeyF8, e.togglePause)
	e.hotkeyMgr.Register(input.KeyF9, e.restart)

	return e
}

// RunMenu 메인 메뉴 실행
func (e *Engine) RunMenu() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println()
		fmt.Println("=== 카카오톡 검키우기 ===")
		fmt.Println("1. 강화 목표 달성")
		fmt.Println("2. 히든 검 뽑기")
		fmt.Println("3. 골드 채굴 (돈벌기)")
		fmt.Println("4. 옵션 설정")
		fmt.Println("0. 종료")
		fmt.Println()
		fmt.Print("선택: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			e.runEnhanceMode(reader)
		case "2":
			e.runHiddenMode()
		case "3":
			e.runGoldMineMode()
		case "4":
			e.showSettings(reader)
		case "0":
			fmt.Println("프로그램을 종료합니다.")
			return
		default:
			fmt.Println("잘못된 입력입니다.")
		}
	}
}

func (e *Engine) runEnhanceMode(reader *bufio.Reader) {
	fmt.Print("목표 강화 레벨 (+숫자): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "+")

	target, err := strconv.Atoi(input)
	if err != nil || target < 1 || target > 15 {
		fmt.Println("잘못된 레벨입니다. (1-15)")
		return
	}

	e.targetLevel = target
	e.mode = ModeEnhance
	e.setupAndRun()
}

func (e *Engine) runHiddenMode() {
	e.mode = ModeHidden
	e.setupAndRun()
}

func (e *Engine) runGoldMineMode() {
	e.targetLevel = e.cfg.GoldMineTarget
	e.mode = ModeGoldMine
	e.setupAndRun()
}

func (e *Engine) setupAndRun() {
	// 좌표 설정
	if !e.cfg.LockXY || e.cfg.ClickX == 0 {
		fmt.Println()
		fmt.Println("카카오톡 메시지 입력창에 마우스를 올려놓으세요...")
		fmt.Println("3초 후 자동으로 좌표를 저장합니다.")

		time.Sleep(3 * time.Second)

		e.cfg.ClickX, e.cfg.ClickY = input.GetMousePos()
		e.cfg.Save()

		fmt.Printf("좌표 저장됨: (%d, %d)\n", e.cfg.ClickX, e.cfg.ClickY)
	}

	// OCR 초기화
	fmt.Println("OCR 엔진 초기화 중...")
	if err := ocr.Init(); err != nil {
		logger.Error("OCR 초기화 실패: %v", err)
		fmt.Printf("OCR 초기화 실패: %v\n", err)
		return
	}

	// 핫키 시작
	e.hotkeyMgr.Start()
	defer e.hotkeyMgr.Stop()

	fmt.Println()
	fmt.Println("=== 매크로 시작 ===")
	fmt.Println("F8: 일시정지/재개")
	fmt.Println("F9: 재시작 (메뉴로)")
	fmt.Println("마우스 좌상단: 비상정지")
	fmt.Println()

	e.running = true
	e.paused = false
	e.cycleCount = 0
	e.totalGold = 0

	// 모드별 실행
	switch e.mode {
	case ModeEnhance:
		e.loopEnhance()
	case ModeHidden:
		e.loopHidden()
	case ModeGoldMine:
		e.loopGoldMine()
	}
}

func (e *Engine) loopEnhance() {
	for e.running {
		if e.checkStop() {
			return
		}

		// 현재 상태 읽기
		state := e.readGameState()
		if state == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 목표 달성 확인
		if state.Level >= e.targetLevel {
			fmt.Printf("\n🎉 목표 달성! +%d\n", state.Level)
			logger.Info("목표 달성: +%d", state.Level)
			return
		}

		// 강화 명령
		e.sendCommand("/강화")
		e.waitForResult(state.Level)
	}
}

func (e *Engine) loopHidden() {
	for e.running {
		if e.checkStop() {
			return
		}

		// 파밍
		e.sendCommand("/파밍")
		time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))

		// OCR로 결과 확인
		state := e.readGameState()
		if state != nil && state.ItemType == "hidden" {
			fmt.Println("\n🎉 히든 아이템 발견!")
			logger.Info("히든 아이템 발견")
			return
		}

		// 트래시면 판매
		if state != nil && state.ItemType == "trash" {
			e.sendCommand("/판매")
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (e *Engine) loopGoldMine() {
	for e.running {
		e.cycleStartTime = time.Now()
		e.cycleCount++

		// 1. 파밍
		if !e.farmUntilHidden() {
			continue
		}

		// 2. 강화
		startGold := e.readCurrentGold()
		if !e.enhanceToTarget() {
			continue
		}

		// 3. 판매
		e.sendCommand("/판매")
		time.Sleep(500 * time.Millisecond)

		// 4. 사이클 통계
		endGold := e.readCurrentGold()
		cycleTime := time.Since(e.cycleStartTime)
		goldEarned := endGold - startGold
		e.totalGold += goldEarned

		fmt.Printf("📦 사이클 #%d: %.1f초, %+dG | 누적: %dG\n",
			e.cycleCount, cycleTime.Seconds(), goldEarned, e.totalGold)
	}
}

func (e *Engine) farmUntilHidden() bool {
	for e.running {
		if e.checkStop() {
			return false
		}

		e.sendCommand("/파밍")
		time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))

		state := e.readGameState()
		if state != nil {
			if state.ItemType == "hidden" {
				return true
			}
			if state.ItemType == "trash" {
				e.sendCommand("/판매")
				time.Sleep(300 * time.Millisecond)
			}
		}
	}
	return false
}

func (e *Engine) enhanceToTarget() bool {
	currentLevel := 0

	for currentLevel < e.targetLevel && e.running {
		if e.checkStop() {
			return false
		}

		e.sendCommand("/강화")
		delay := e.getDelayForLevel(currentLevel)
		time.Sleep(delay)

		state := e.readGameState()
		if state == nil {
			continue
		}

		switch state.LastResult {
		case "success":
			currentLevel++
			fmt.Printf("  ✅ +%d 성공\n", currentLevel)
		case "destroy":
			fmt.Println("  💥 파괴!")
			return false
		case "hold":
			fmt.Printf("  ⏸️ +%d 유지\n", currentLevel)
		}
	}

	return currentLevel >= e.targetLevel
}

func (e *Engine) getDelayForLevel(level int) time.Duration {
	var delay float64
	switch {
	case level < 5:
		delay = e.cfg.LowDelay
	case level < e.cfg.SlowdownLevel:
		delay = e.cfg.MidDelay
	default:
		delay = e.cfg.HighDelay
	}
	return time.Duration(delay * float64(time.Second))
}

func (e *Engine) readGameState() *GameState {
	// 화면 캡처
	x := e.cfg.ClickX - e.cfg.CaptureW/2
	y := e.cfg.ClickY - e.cfg.InputBoxH/2 - e.cfg.CaptureH

	img, err := capture.CaptureRegion(x, y, e.cfg.CaptureW, e.cfg.CaptureH)
	if err != nil {
		logger.Error("캡처 실패: %v", err)
		return nil
	}

	// OCR
	text, err := ocr.Recognize(img)
	if err != nil {
		logger.Error("OCR 실패: %v", err)
		return nil
	}

	logger.OCR(text)
	return ParseOCRText(text)
}

func (e *Engine) readCurrentGold() int {
	state := e.readGameState()
	if state != nil && state.Gold > 0 {
		return state.Gold
	}
	return 0
}

func (e *Engine) waitForResult(prevLevel int) {
	delay := e.getDelayForLevel(prevLevel)
	time.Sleep(delay)
}

func (e *Engine) sendCommand(cmd string) {
	input.SendCommand(e.cfg.ClickX, e.cfg.ClickY, cmd)
}

func (e *Engine) checkStop() bool {
	// 비상 정지 체크
	if input.CheckFailsafe() {
		fmt.Println("\n⚠️ 비상 정지!")
		e.running = false
		return true
	}

	// 일시정지 체크
	for e.paused && e.running {
		time.Sleep(100 * time.Millisecond)
	}

	return !e.running
}

func (e *Engine) togglePause() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.paused = !e.paused
	if e.paused {
		fmt.Println("\n⏸️ 일시정지 (F8로 재개)")
	} else {
		fmt.Println("\n▶️ 재개")
	}
}

func (e *Engine) restart() {
	e.mu.Lock()
	defer e.mu.Unlock()

	fmt.Println("\n🔄 재시작...")
	e.running = false
}

// Stop 엔진 정지
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = false
}

func (e *Engine) showSettings(reader *bufio.Reader) {
	for {
		fmt.Println()
		fmt.Println("=== 옵션 설정 ===")
		fmt.Printf("1. 감속 시작 레벨: +%d\n", e.cfg.SlowdownLevel)
		fmt.Printf("2. 중간 속도: %.1f초\n", e.cfg.MidDelay)
		fmt.Printf("3. 고강 속도: %.1f초\n", e.cfg.HighDelay)
		fmt.Printf("4. 좌표 고정: %v\n", e.cfg.LockXY)
		fmt.Printf("5. 골드 채굴 목표: +%d\n", e.cfg.GoldMineTarget)
		fmt.Println("0. 돌아가기")
		fmt.Print("선택: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			fmt.Print("감속 시작 레벨 (1-15): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 1 && v <= 15 {
				e.cfg.SlowdownLevel = v
			}
		case "2":
			fmt.Print("중간 속도 (초): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && v > 0 {
				e.cfg.MidDelay = v
			}
		case "3":
			fmt.Print("고강 속도 (초): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && v > 0 {
				e.cfg.HighDelay = v
			}
		case "4":
			e.cfg.LockXY = !e.cfg.LockXY
			fmt.Printf("좌표 고정: %v\n", e.cfg.LockXY)
		case "5":
			fmt.Print("골드 채굴 목표 레벨 (1-15): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 1 && v <= 15 {
				e.cfg.GoldMineTarget = v
			}
		case "0":
			e.cfg.Save()
			return
		}
	}
}
