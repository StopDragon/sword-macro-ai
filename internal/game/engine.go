package game

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/StopDragon/sword-macro-ai/internal/analysis"
	"github.com/StopDragon/sword-macro-ai/internal/capture"
	"github.com/StopDragon/sword-macro-ai/internal/config"
	"github.com/StopDragon/sword-macro-ai/internal/input"
	"github.com/StopDragon/sword-macro-ai/internal/logger"
	"github.com/StopDragon/sword-macro-ai/internal/ocr"
	"github.com/StopDragon/sword-macro-ai/internal/overlay"
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
	myProfile    *Profile
	battleWins   int
	battleLosses int

	// 핫키
	hotkeyMgr *input.HotkeyManager

	// v2: 세션 분석 및 알림
	session *analysis.SessionTracker
	alerts  *analysis.AlertEngine

	// 세션 통계 (종료 시 출력용)
	sessionStats struct {
		startGold       int
		endGold         int
		trashCount      int
		hiddenCount     int
		enhanceSuccess  int
		enhanceHold     int
		enhanceDestroy  int
		cycleTimeSum    float64 // 사이클 시간 합계 (초)
		cycleGoldSum    int     // 사이클 수익 합계
	}
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

	// OCR 캡처 영역 및 입력창 영역 계산
	captureX := e.cfg.ClickX - e.cfg.CaptureW/2
	captureY := e.cfg.ClickY - e.cfg.InputBoxH/2 - e.cfg.CaptureH
	inputX := e.cfg.ClickX - 150 // 입력창 너비 추정
	inputY := e.cfg.ClickY - e.cfg.InputBoxH/2
	inputW := 300
	inputH := e.cfg.InputBoxH

	fmt.Println()
	fmt.Println("🔴 빨간 테두리 = OCR 캡처 영역 (채팅 내용)")
	fmt.Println("🟢 초록 테두리 = 입력창 영역 (명령어 입력)")
	fmt.Printf("   OCR: (%d, %d) ~ (%d, %d)\n", captureX, captureY, captureX+e.cfg.CaptureW, captureY+e.cfg.CaptureH)
	fmt.Printf("   입력: (%d, %d) ~ (%d, %d)\n", inputX, inputY, inputX+inputW, inputY+inputH)

	// 모든 오버레이 표시 (OCR 영역 + 입력창 영역 + 상태 패널)
	overlay.ShowAll(captureX, captureY, e.cfg.CaptureW, e.cfg.CaptureH, inputX, inputY, inputW, inputH)
	overlay.UpdateStatus("🎮 준비 중...\n카카오톡 창을\n오버레이에 맞춰주세요")

	fmt.Println()
	fmt.Println("⚠️  카카오톡 채팅창을 오버레이에 맞춰 배치하세요!")
	fmt.Println()

	// 5초 대기
	fmt.Print("⏳ 준비 대기: ")
	for i := 5; i > 0; i-- {
		fmt.Printf("%d... ", i)
		overlay.UpdateStatus("🎮 준비 중... %d초\n카카오톡 창을\n오버레이에 맞춰주세요", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("시작!")
	overlay.UpdateStatus("🚀 시작!")

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

	// 세션 통계 초기화
	e.sessionStats.startGold = e.readCurrentGold()
	e.sessionStats.endGold = 0
	e.sessionStats.trashCount = 0
	e.sessionStats.hiddenCount = 0
	e.sessionStats.enhanceSuccess = 0
	e.sessionStats.enhanceHold = 0
	e.sessionStats.enhanceDestroy = 0
	e.sessionStats.cycleTimeSum = 0
	e.sessionStats.cycleGoldSum = 0

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

	// 종료 시 오버레이 숨기기
	overlay.UpdateStatus("⏹️ 종료 중...")
	time.Sleep(500 * time.Millisecond)
	overlay.HideAll()

	// 종료 시 현재 골드 읽기
	e.sessionStats.endGold = e.readCurrentGold()

	// 상세 통계 출력
	e.printSessionStats()

	// 텔레메트리 전송
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

// printSessionStats 세션 종료 시 상세 통계 출력
func (e *Engine) printSessionStats() {
	elapsed := time.Since(e.startTime)
	elapsedSec := elapsed.Seconds()

	// 골드 변화 계산
	goldDiff := e.sessionStats.endGold - e.sessionStats.startGold
	if e.sessionStats.startGold <= 0 {
		goldDiff = e.totalGold // 시작 골드를 못 읽었으면 누적 수익 사용
	}

	// 시간당 골드 계산
	goldPerHour := 0
	if elapsedSec > 0 {
		goldPerHour = int(float64(goldDiff) / elapsedSec * 3600)
	}

	// 사이클 평균 계산
	avgCycleTime := 0.0
	avgCycleGold := 0
	if e.cycleCount > 0 {
		avgCycleTime = e.sessionStats.cycleTimeSum / float64(e.cycleCount)
		avgCycleGold = e.sessionStats.cycleGoldSum / e.cycleCount
	}

	// 골드 부호
	goldSign := "+"
	if goldDiff < 0 {
		goldSign = ""
	}
	gphSign := "+"
	if goldPerHour < 0 {
		gphSign = ""
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  📊 세션 통계 (%s)\n", formatDuration(elapsed))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 파밍 통계
	if e.sessionStats.trashCount > 0 || e.sessionStats.hiddenCount > 0 {
		fmt.Printf("  🎣 트래시 판매: %d회\n", e.sessionStats.trashCount)
		fmt.Printf("  ⭐ 히든 발견:   %d회\n", e.sessionStats.hiddenCount)
	}

	// 강화 통계
	enhanceTotal := e.sessionStats.enhanceSuccess + e.sessionStats.enhanceHold + e.sessionStats.enhanceDestroy
	if enhanceTotal > 0 {
		fmt.Printf("  ✅ 강화 성공:   %d회\n", e.sessionStats.enhanceSuccess)
		fmt.Printf("  ⏸️  강화 유지:   %d회\n", e.sessionStats.enhanceHold)
		fmt.Printf("  💥 강화 파괴:   %d회\n", e.sessionStats.enhanceDestroy)
	}

	// 배틀 통계
	if e.battleWins > 0 || e.battleLosses > 0 {
		winRate := 0.0
		if e.battleWins+e.battleLosses > 0 {
			winRate = float64(e.battleWins) / float64(e.battleWins+e.battleLosses) * 100
		}
		fmt.Printf("  ⚔️  배틀 전적:   %d승 %d패 (%.1f%%)\n", e.battleWins, e.battleLosses, winRate)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 골드 통계
	if e.sessionStats.startGold > 0 && e.sessionStats.endGold > 0 {
		fmt.Printf("  💰 골드 변화:   %sG → %sG (%s%sG)\n",
			FormatGold(e.sessionStats.startGold),
			FormatGold(e.sessionStats.endGold),
			goldSign, FormatGold(goldDiff))
	} else if e.totalGold != 0 {
		fmt.Printf("  💰 총 수익:     %s%sG\n", goldSign, FormatGold(goldDiff))
	}

	fmt.Printf("  📈 시간당 골드: %s%sG/h\n", gphSign, FormatGold(goldPerHour))

	// 사이클 통계
	if e.cycleCount > 0 {
		avgGoldSign := "+"
		if avgCycleGold < 0 {
			avgGoldSign = ""
		}
		fmt.Printf("  🔄 완료 사이클: %d회 (평균 %.0f초, %s%sG/사이클)\n",
			e.cycleCount, avgCycleTime, avgGoldSign, FormatGold(avgCycleGold))
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
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

		// 1. /판매 시도 (현재 검 팔고 새 검 받기)
		e.sendCommand("/판매")
		time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))

		// OCR로 결과 확인
		text := e.readOCRText()

		// 2. 판매 불가 체크 (0강 아이템)
		if CannotSell(text) {
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		state := ParseOCRText(text)
		if state == nil {
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// v2: 아이템 이름 추출
		itemName := state.ItemName
		if itemName == "" {
			itemName = ExtractItemName(text)
		}

		// 3. 히든이면 성공
		if state.ItemType == "hidden" {
			fmt.Printf("\n🎉 히든 아이템 발견! [%s]\n", itemName)
			logger.Info("히든 아이템 발견: %s", itemName)

			// v2 텔레메트리: 아이템 이름 포함
			e.telem.RecordFarmingWithItem(itemName, "hidden")
			e.telem.RecordSword()
			e.telem.TrySend()
			e.sessionStats.hiddenCount++
			return
		}

		// 4. 트래시/일반이면 /강화로 파괴
		if state.ItemType == "trash" || state.ItemType == "normal" || state.ItemType == "unknown" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
		}
	}
}

func (e *Engine) loopGoldMine() {
	// v2: 세션 초기화
	startGold := e.readCurrentGold()
	e.telem.InitSession(startGold)
	overlay.UpdateStatus("💰 골드 채굴 모드\n사이클: 0\n수익: 0G")

	for e.running {
		e.cycleStartTime = time.Now()
		e.cycleCount++

		// 1. 파밍 (아이템 이름 반환)
		overlay.UpdateStatus("💰 골드 채굴 #%d\n🔍 파밍 중...\n누적: %sG", e.cycleCount, FormatGold(e.totalGold))
		itemName, found := e.farmUntilHiddenWithName()
		if !found {
			e.telem.RecordCycle(false)
			overlay.UpdateStatus("💰 골드 채굴 #%d\n❌ 파밍 실패\n누적: %sG", e.cycleCount, FormatGold(e.totalGold))
			continue
		}

		// 2. 강화
		overlay.UpdateStatus("💰 골드 채굴 #%d\n⚔️ 강화 중: %s\n누적: %sG", e.cycleCount, itemName, FormatGold(e.totalGold))
		cycleStartGold := e.readCurrentGold()
		finalLevel, success := e.enhanceToTargetWithLevel()
		if !success {
			e.telem.RecordCycle(false)
			continue
		}

		// 3. 판매
		overlay.UpdateStatus("💰 골드 채굴 #%d\n💵 판매 중: %s +%d\n누적: %sG", e.cycleCount, itemName, finalLevel, FormatGold(e.totalGold))
		e.sendCommand("/판매")
		time.Sleep(500 * time.Millisecond)

		// 4. 사이클 통계
		endGold := e.readCurrentGold()
		cycleTime := time.Since(e.cycleStartTime)
		goldEarned := endGold - cycleStartGold
		e.totalGold += goldEarned

		// v2 텔레메트리 기록
		e.telem.RecordCycle(true)
		e.telem.RecordGold(goldEarned)
		e.telem.RecordSaleWithSword(itemName, finalLevel, goldEarned)
		e.telem.RecordGoldChange(endGold)
		e.telem.TrySend()

		// 세션 통계 업데이트
		e.sessionStats.cycleTimeSum += cycleTime.Seconds()
		e.sessionStats.cycleGoldSum += goldEarned

		// 사이클 완료 상태 업데이트
		overlay.UpdateStatus("💰 골드 채굴 #%d ✅\n%s +%d → %+sG\n누적: %sG", e.cycleCount, itemName, finalLevel, FormatGold(goldEarned), FormatGold(e.totalGold))

		fmt.Printf("📦 사이클 #%d: %.1f초, %+dG | 누적: %dG [%s +%d]\n",
			e.cycleCount, cycleTime.Seconds(), goldEarned, e.totalGold, itemName, finalLevel)
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

	// v2: 세션 초기화
	startGold := e.readCurrentGold()
	e.telem.InitSession(startGold)

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
		fmt.Printf("⚔️ #%d: %s (+%d) vs 나 (+%d) [%s]\n",
			e.cycleCount, target.Username, target.Level, e.myProfile.Level, e.myProfile.SwordName)

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

		// 5. v2 텔레메트리 기록 (검 이름 포함)
		e.telem.RecordBattleWithSword(e.myProfile.SwordName, e.myProfile.Level, target.Level, result.Won, goldEarned)
		e.telem.RecordGoldChange(e.readCurrentGold())
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
	_, found := e.farmUntilHiddenWithName()
	return found
}

// farmUntilHiddenWithName 히든 아이템을 찾을 때까지 파밍하고 아이템 이름 반환
// 로직: /판매 → OCR → 트래시면 /강화(파괴) → 반복, 히든이면 반환
func (e *Engine) farmUntilHiddenWithName() (string, bool) {
	for e.running {
		if e.checkStop() {
			return "", false
		}

		// 1. /판매 시도 (현재 검 팔고 새 검 받기)
		e.sendCommand("/판매")
		time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))

		text := e.readOCRText()

		// 2. 판매 불가 체크 (0강 아이템은 판매 불가)
		if CannotSell(text) {
			// 0강 아이템은 /강화로 파괴
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// 3. 새 검 획득 체크
		state := ParseOCRText(text)
		if state == nil {
			// OCR 실패 시 /강화 시도
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// v2: 아이템 이름 추출
		itemName := state.ItemName
		if itemName == "" {
			itemName = ExtractItemName(text)
		}

		// 4. 히든 아이템이면 반환 (강화 모드로 전환)
		if state.ItemType == "hidden" {
			e.telem.RecordFarmingWithItem(itemName, "hidden")
			e.sessionStats.hiddenCount++
			fmt.Printf("🎉 히든 발견! [%s]\n", itemName)
			return itemName, true
		}

		// 5. 트래시/일반 아이템이면 /강화로 파괴하고 반복
		if state.ItemType == "trash" || state.ItemType == "normal" || state.ItemType == "unknown" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			// 트래시는 /강화로 파괴 (0강이므로 바로 파괴됨)
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
		}
	}
	return "", false
}

func (e *Engine) enhanceToTarget() bool {
	_, success := e.enhanceToTargetWithLevel()
	return success
}

// enhanceToTargetWithLevel 목표까지 강화하고 최종 레벨 반환
func (e *Engine) enhanceToTargetWithLevel() (int, bool) {
	currentLevel := 0

	for currentLevel < e.targetLevel && e.running {
		if e.checkStop() {
			return currentLevel, false
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
			e.telem.RecordEnhance(currentLevel-1, "success")
			e.sessionStats.enhanceSuccess++
		case "destroy":
			fmt.Println("  💥 파괴!")
			e.telem.RecordEnhance(currentLevel, "destroy")
			e.sessionStats.enhanceDestroy++
			return currentLevel, false
		case "hold":
			fmt.Printf("  ⏸️ +%d 유지\n", currentLevel)
			e.telem.RecordEnhance(currentLevel, "hold")
			e.sessionStats.enhanceHold++
		}
	}

	return currentLevel, currentLevel >= e.targetLevel
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

	// 좌표 설정
	if !e.cfg.LockXY || e.cfg.ClickX == 0 {
		fmt.Println("카카오톡 메시지 입력창에 마우스를 올려놓으세요...")
		fmt.Println("3초 후 좌표를 저장합니다.")
		for i := 3; i > 0; i-- {
			fmt.Printf("\r%d...", i)
			time.Sleep(1 * time.Second)
		}
		fmt.Println()
		e.cfg.ClickX, e.cfg.ClickY = input.GetMousePos()
		e.cfg.Save()
		fmt.Printf("✅ 좌표 저장됨: (%d, %d)\n", e.cfg.ClickX, e.cfg.ClickY)
	} else {
		fmt.Printf("📍 저장된 좌표 사용: (%d, %d)\n", e.cfg.ClickX, e.cfg.ClickY)
	}

	// OCR 캡처 영역 표시
	captureX := e.cfg.ClickX - e.cfg.CaptureW/2
	captureY := e.cfg.ClickY - e.cfg.InputBoxH/2 - e.cfg.CaptureH
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────┐")
	fmt.Println("│          📸 OCR 캡처 영역               │")
	fmt.Println("├─────────────────────────────────────────┤")
	fmt.Printf("│  좌상단: (%d, %d)                      \n", captureX, captureY)
	fmt.Printf("│  우하단: (%d, %d)                      \n", captureX+e.cfg.CaptureW, captureY+e.cfg.CaptureH)
	fmt.Printf("│  크기: %d x %d                         \n", e.cfg.CaptureW, e.cfg.CaptureH)
	fmt.Println("└─────────────────────────────────────────┘")
	fmt.Println()

	// 오버레이 표시
	fmt.Println("🔴 빨간색 테두리가 OCR 캡처 영역입니다!")
	overlay.Show(captureX, captureY, e.cfg.CaptureW, e.cfg.CaptureH)

	fmt.Println("⚠️  카카오톡 채팅창을 빨간 테두리 안에 맞춰 배치하세요!")
	fmt.Println("    (프로필 응답이 빨간 영역 안에 보여야 합니다)")
	fmt.Println()

	// 5초 대기 (사용자가 카톡 창을 OCR 영역으로 이동할 시간)
	fmt.Print("⏳ 준비 대기: ")
	for i := 5; i > 0; i-- {
		fmt.Printf("%d... ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("시작!")
	fmt.Println()

	// 오버레이 숨기기
	overlay.Hide()

	// OCR 초기화
	fmt.Println("🔧 OCR 엔진 초기화 중...")
	if err := ocr.Init(); err != nil {
		fmt.Printf("❌ OCR 초기화 실패: %v\n", err)
		return
	}
	fmt.Println("✅ OCR 준비 완료")

	// /프로필 명령어 전송
	fmt.Println()
	fmt.Println("📤 /프로필 명령어 전송 중...")
	e.sendCommand("/프로필")
	fmt.Println("⏳ 응답 대기 중 (2초)...")
	time.Sleep(2 * time.Second)

	// OCR로 프로필 읽기
	fmt.Println("🔍 화면 캡처 및 OCR 분석 중...")
	profileText := e.readOCRText()

	// 디버그: OCR 결과 출력
	if profileText == "" {
		fmt.Println("⚠️ OCR 결과가 비어있습니다.")
		fmt.Println()
		fmt.Println("🔧 문제 해결 방법:")
		fmt.Println("   1. 카카오톡 창이 화면에 보이는지 확인")
		fmt.Println("   2. 메시지 입력창 위치가 맞는지 확인")
		fmt.Printf("   3. 캡처 영역 확인: (%d, %d) ~ (%d, %d)\n",
			captureX, captureY, captureX+e.cfg.CaptureW, captureY+e.cfg.CaptureH)
		fmt.Println("   4. 좌표 고정 해제 후 다시 시도 (옵션 설정 → 좌표 고정)")
		return
	}

	profile := ParseProfile(profileText)

	if profile == nil || profile.Level < 0 {
		fmt.Println("❌ 프로필을 파싱할 수 없습니다.")
		fmt.Println()
		fmt.Println("📝 OCR 인식된 텍스트 (처음 200자):")
		preview := profileText
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Printf("   %s\n", preview)
		fmt.Println()
		fmt.Println("🔧 문제 해결 방법:")
		fmt.Println("   1. /프로필 명령어가 제대로 전송되었는지 확인")
		fmt.Println("   2. 카카오톡에서 프로필 응답이 표시되는지 확인")
		fmt.Println("   3. 메시지 입력창 위치를 다시 설정해보세요")
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
