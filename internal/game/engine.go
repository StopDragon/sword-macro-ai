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
	"github.com/StopDragon/sword-macro-ai/internal/telemetry"
)

// Mode 매크로 모드
type Mode int

const (
	ModeNone Mode = iota
	ModeEnhance  // 강화 목표 달성
	ModeHidden   // 히든 검 뽑기
	ModeGoldMine // 골드 채굴
	ModeBattle   // 자동 배틀 (역배)
)

// Engine 게임 엔진
type Engine struct {
	cfg       *config.Config
	telem     *telemetry.Telemetry
	mode      Mode
	running   bool
	paused    bool
	mu        sync.Mutex

	// 상태
	currentLevel   int
	targetLevel    int
	cycleCount     int
	cycleStartTime time.Time
	totalGold      int

	// 실행 시간 제한
	duration  time.Duration
	startTime time.Time
	stopTimer *time.Timer

	// 배틀 상태
	myProfile   *Profile
	battleWins  int
	battleLosses int

	// 핫키
	hotkeyMgr *input.HotkeyManager
}

// NewEngine 엔진 생성
func NewEngine(cfg *config.Config, telem *telemetry.Telemetry) *Engine {
	e := &Engine{
		cfg:   cfg,
		telem: telem,
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
		fmt.Println("4. 자동 배틀 (역배)")
		fmt.Println("5. 옵션 설정")
		fmt.Println("6. 내 프로필 분석")
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
			e.runBattleMode(reader)
		case "5":
			e.showSettings(reader)
		case "6":
			e.showMyProfile()
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

func (e *Engine) runBattleMode(reader *bufio.Reader) {
	fmt.Println()
	fmt.Println("=== 자동 배틀 설정 ===")
	fmt.Printf("현재 역배 레벨 차이: %d (내 레벨 +1 ~ +%d 상대와 대결)\n",
		e.cfg.BattleLevelDiff, e.cfg.BattleLevelDiff)

	fmt.Print("역배 레벨 차이 (1-3, 엔터=유지): ")
	diffInput, _ := reader.ReadString('\n')
	diffInput = strings.TrimSpace(diffInput)
	if diff, err := strconv.Atoi(diffInput); err == nil && diff >= 1 && diff <= 3 {
		e.cfg.BattleLevelDiff = diff
		e.cfg.Save()
	}

	e.mode = ModeBattle
	e.battleWins = 0
	e.battleLosses = 0
	e.setupAndRun()
}

func (e *Engine) setupAndRun() {
	// 실행 시간 설정
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("몇 분간 진행할까요? (0 = 무제한): ")
	durInput, _ := reader.ReadString('\n')
	durInput = strings.TrimSpace(durInput)

	if minutes, err := strconv.Atoi(durInput); err == nil && minutes > 0 {
		e.duration = time.Duration(minutes) * time.Minute
		fmt.Printf("⏱️ %d분 후 자동 종료됩니다.\n", minutes)
	} else {
		e.duration = 0
		fmt.Println("⏱️ 무제한 모드 (수동 종료)")
	}

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
	e.startTime = time.Now()

	// 타이머 설정 (시간 제한이 있는 경우)
	if e.duration > 0 {
		e.stopTimer = time.AfterFunc(e.duration, func() {
			fmt.Printf("\n\n⏰ %d분 경과! 자동 종료합니다...\n", int(e.duration.Minutes()))
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
		})
		defer e.stopTimer.Stop()
	}

	// 모드별 실행
	switch e.mode {
	case ModeEnhance:
		e.loopEnhance()
	case ModeHidden:
		e.loopHidden()
	case ModeGoldMine:
		e.loopGoldMine()
	case ModeBattle:
		e.loopBattle()
	}

	// 종료 시 통계 출력 및 텔레메트리 전송
	elapsed := time.Since(e.startTime)
	fmt.Println()
	fmt.Println("=== 매크로 종료 ===")
	fmt.Printf("⏱️ 실행 시간: %s\n", formatDuration(elapsed))
	fmt.Printf("🔄 총 사이클: %d회\n", e.cycleCount)
	if e.totalGold > 0 {
		fmt.Printf("💰 총 수익: %dG\n", e.totalGold)
	}
	fmt.Println("📤 통계 전송 중...")
	e.telem.Flush()
	fmt.Println("✅ 완료!")
}

// formatDuration 시간을 읽기 쉽게 포맷
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%d시간 %d분 %d초", h, m, s)
	} else if m > 0 {
		return fmt.Sprintf("%d분 %d초", m, s)
	}
	return fmt.Sprintf("%d초", s)
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
			e.telem.RecordSword()
			e.telem.TrySend()
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
			e.telem.RecordSword()
			e.telem.TrySend()
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
			e.telem.RecordCycle(false)
			continue
		}

		// 2. 강화
		startGold := e.readCurrentGold()
		if !e.enhanceToTarget() {
			e.telem.RecordCycle(false)
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

		// 텔레메트리 기록
		e.telem.RecordCycle(true)
		e.telem.RecordGold(goldEarned)
		e.telem.TrySend()

		fmt.Printf("📦 사이클 #%d: %.1f초, %+dG | 누적: %dG\n",
			e.cycleCount, cycleTime.Seconds(), goldEarned, e.totalGold)
	}
}

func (e *Engine) loopBattle() {
	fmt.Println()
	fmt.Println("📊 프로필 확인 중...")

	// 1. 내 프로필 확인
	e.sendCommand("/프로필")
	time.Sleep(2 * time.Second)

	profileText := e.readOCRText()
	e.myProfile = ParseProfile(profileText)

	if e.myProfile == nil || e.myProfile.Level < 0 {
		fmt.Println("❌ 프로필을 읽을 수 없습니다. 다시 시도하세요.")
		return
	}

	fmt.Printf("📋 내 프로필: +%d %s (%d승 %d패)\n",
		e.myProfile.Level, e.myProfile.SwordName, e.myProfile.Wins, e.myProfile.Losses)
	fmt.Printf("🎯 타겟 범위: +%d ~ +%d\n",
		e.myProfile.Level+1, e.myProfile.Level+e.cfg.BattleLevelDiff)
	fmt.Println()

	// 배틀 루프
	for e.running {
		if e.checkStop() {
			return
		}

		e.cycleCount++

		// 2. 랭킹에서 타겟 찾기
		e.sendCommand("/랭킹")
		time.Sleep(2 * time.Second)

		rankingText := e.readOCRText()
		entries := ParseRanking(rankingText)
		targets := FindTargetsInRanking(entries, e.myProfile.Level, e.cfg.BattleLevelDiff)

		if len(targets) == 0 {
			fmt.Println("⏳ 적합한 타겟 없음, 30초 후 재시도...")
			time.Sleep(30 * time.Second)
			continue
		}

		// 3. 첫 번째 타겟과 배틀
		target := targets[0]
		fmt.Printf("⚔️ #%d: %s (+%d) vs 나 (+%d)\n",
			e.cycleCount, target.Username, target.Level, e.myProfile.Level)

		e.sendCommand("/배틀 " + target.Username)
		time.Sleep(3 * time.Second)

		// 4. 결과 확인
		resultText := e.readOCRText()
		result := ParseBattleResult(resultText, e.myProfile.Name)

		goldEarned := 0
		if result.Won {
			e.battleWins++
			goldEarned = result.GoldEarned
			e.totalGold += goldEarned
			fmt.Printf("   → 🏆 승리! +%dG (역배 성공!)\n", goldEarned)
		} else {
			e.battleLosses++
			fmt.Println("   → 💔 패배...")
		}

		// 5. 텔레메트리 기록
		e.telem.RecordBattle(e.myProfile.Level, target.Level, result.Won, goldEarned)
		e.telem.TrySend()

		// 6. 현재 통계 출력
		winRate := float64(0)
		if e.battleWins+e.battleLosses > 0 {
			winRate = float64(e.battleWins) / float64(e.battleWins+e.battleLosses) * 100
		}
		fmt.Printf("   📊 전적: %d승 %d패 (%.1f%%) | 수익: %dG\n",
			e.battleWins, e.battleLosses, winRate, e.totalGold)

		// 7. 골드 체크
		currentGold := e.readCurrentGold()
		if currentGold > 0 && currentGold < e.cfg.BattleMinGold {
			fmt.Printf("⚠️ 골드 부족! (%dG < %dG) 배틀 중단\n", currentGold, e.cfg.BattleMinGold)
			return
		}

		// 8. 프로필 갱신 (레벨 변동 확인)
		e.sendCommand("/프로필")
		time.Sleep(1 * time.Second)
		profileText = e.readOCRText()
		newProfile := ParseProfile(profileText)
		if newProfile != nil && newProfile.Level > 0 {
			e.myProfile = newProfile
		}

		// 9. 쿨다운
		time.Sleep(time.Duration(e.cfg.BattleCooldown * float64(time.Second)))
	}
}

// readOCRText 화면에서 OCR 텍스트 읽기
func (e *Engine) readOCRText() string {
	x := e.cfg.ClickX - e.cfg.CaptureW/2
	y := e.cfg.ClickY - e.cfg.InputBoxH/2 - e.cfg.CaptureH

	img, err := capture.CaptureRegion(x, y, e.cfg.CaptureW, e.cfg.CaptureH)
	if err != nil {
		logger.Error("캡처 실패: %v", err)
		return ""
	}

	text, err := ocr.Recognize(img)
	if err != nil {
		logger.Error("OCR 실패: %v", err)
		return ""
	}

	logger.OCR(text)
	return text
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
		fmt.Printf("6. 배틀 역배 레벨차: %d\n", e.cfg.BattleLevelDiff)
		fmt.Printf("7. 배틀 쿨다운: %.1f초\n", e.cfg.BattleCooldown)
		fmt.Printf("8. 배틀 최소 골드: %dG\n", e.cfg.BattleMinGold)
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
		case "6":
			fmt.Print("배틀 역배 레벨차 (1-3): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 1 && v <= 3 {
				e.cfg.BattleLevelDiff = v
			}
		case "7":
			fmt.Print("배틀 쿨다운 (초): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && v > 0 {
				e.cfg.BattleCooldown = v
			}
		case "8":
			fmt.Print("배틀 최소 골드: ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 0 {
				e.cfg.BattleMinGold = v
			}
		case "0":
			e.cfg.Save()
			return
		}
	}
}

func (e *Engine) showMyProfile() {
	fmt.Println()
	fmt.Println("=== 내 프로필 분석 ===")
	fmt.Println("카카오톡에서 /프로필을 입력하고")
	fmt.Println("메시지 입력창에 마우스를 올려놓으세요...")
	fmt.Println("3초 후 프로필을 읽습니다.")

	// 좌표 설정
	if !e.cfg.LockXY || e.cfg.ClickX == 0 {
		time.Sleep(3 * time.Second)
		e.cfg.ClickX, e.cfg.ClickY = input.GetMousePos()
		e.cfg.Save()
	}

	// OCR 초기화
	if err := ocr.Init(); err != nil {
		fmt.Printf("❌ OCR 초기화 실패: %v\n", err)
		return
	}

	// /프로필 명령어 전송
	e.sendCommand("/프로필")
	time.Sleep(2 * time.Second)

	// OCR로 프로필 읽기
	profileText := e.readOCRText()
	profile := ParseProfile(profileText)

	if profile == nil || profile.Level < 0 {
		fmt.Println("❌ 프로필을 읽을 수 없습니다.")
		fmt.Println("   카카오톡 창이 보이는지 확인하세요.")
		return
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. 내 검 정보
	fmt.Println("⚔️ 내 검 정보")
	fmt.Printf("   이름: %s\n", profile.Name)
	if profile.SwordName != "" {
		fmt.Printf("   보유 검: [+%d] %s\n", profile.Level, profile.SwordName)
	} else {
		fmt.Printf("   보유 검: +%d\n", profile.Level)
	}
	fmt.Printf("   전적: %d승 %d패\n", profile.Wins, profile.Losses)
	if profile.Gold > 0 {
		fmt.Printf("   보유 골드: %sG\n", FormatGold(profile.Gold))
	}
	fmt.Println()

	// 2. 예상 판매가
	fmt.Println("💰 예상 판매가")
	price := GetSwordPrice(profile.Level)
	if price != nil {
		fmt.Printf("   최소: %sG\n", FormatGold(price.MinPrice))
		fmt.Printf("   평균: %sG\n", FormatGold(price.AvgPrice))
		fmt.Printf("   최대: %sG\n", FormatGold(price.MaxPrice))
	} else {
		fmt.Println("   데이터 없음")
	}
	fmt.Println()

	// 3. 강화 확률표
	fmt.Println("📊 강화 확률 (현재 레벨 기준)")
	fmt.Println("   레벨  | 성공  | 유지  | 파괴  | 예상 판매가")
	fmt.Println("   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 현재 레벨부터 +15까지 표시
	rates := GetAllEnhanceRates()
	for lvl := profile.Level; lvl <= 15 && rates != nil && lvl < len(rates); lvl++ {
		rate := GetEnhanceRate(lvl)
		if rate == nil {
			continue
		}
		nextPrice := GetSwordPrice(lvl + 1)
		priceStr := "-"
		if nextPrice != nil {
			priceStr = FormatGold(nextPrice.AvgPrice)
		}

		marker := "  "
		if lvl == profile.Level {
			marker = "▶ "
		}

		fmt.Printf("   %s+%d→+%d | %4.0f%% | %4.0f%% | %4.0f%% | %s\n",
			marker, lvl, lvl+1, rate.SuccessRate, rate.KeepRate, rate.DestroyRate, priceStr)
	}
	fmt.Println()

	// 4. 목표별 성공 확률
	fmt.Println("🎯 목표 달성 확률")
	targets := []int{profile.Level + 1, profile.Level + 2, profile.Level + 3, 10, 12, 15}
	shown := make(map[int]bool)

	for _, target := range targets {
		if target <= profile.Level || target > 15 || shown[target] {
			continue
		}
		shown[target] = true

		chance := CalcEnhanceSuccessChance(profile.Level, target)
		trials := CalcExpectedTrials(profile.Level, target)
		targetPrice := GetSwordPrice(target)

		priceStr := ""
		if targetPrice != nil {
			priceStr = fmt.Sprintf(" (판매가: %sG)", FormatGold(targetPrice.AvgPrice))
		}

		fmt.Printf("   +%d → +%d: %.2f%% (평균 %.0f회 시도)%s\n",
			profile.Level, target, chance, trials, priceStr)
	}
	fmt.Println()

	// 5. 역배 기대값
	fmt.Printf("⚡ 역배 분석 (내 레벨: +%d)\n", profile.Level)
	fmt.Println("   레벨차 | 승률  | 평균보상 | 기대값")
	fmt.Println("   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	betAmount := 100 // 기본 배팅 금액 가정
	if profile.Gold > 0 {
		betAmount = profile.Gold / 10 // 보유 골드의 10%를 배팅으로 가정
		if betAmount < 100 {
			betAmount = 100
		}
	}

	for diff := 1; diff <= 3; diff++ {
		reward := GetBattleReward(diff)
		if reward == nil {
			continue
		}

		ev, winRate, avgReward := CalcUpsetExpectedValue(profile.Level, profile.Level+diff, betAmount)

		evStr := fmt.Sprintf("%+.0fG", ev)
		if ev > 0 {
			evStr = "🟢 " + evStr
		} else if ev < 0 {
			evStr = "🔴 " + evStr
		}

		fmt.Printf("   +%d     | %4.0f%% | %6sG | %s\n",
			diff, winRate, FormatGold(avgReward), evStr)
	}
	fmt.Println()
	fmt.Printf("   💡 배팅 기준: %sG (보유 골드의 10%%)\n", FormatGold(betAmount))

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
