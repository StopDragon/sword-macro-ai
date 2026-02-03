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
	"github.com/StopDragon/sword-macro-ai/internal/config"
	"github.com/StopDragon/sword-macro-ai/internal/input"
	"github.com/StopDragon/sword-macro-ai/internal/logger"
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

	// 세션 프로필 (필터링용)
	sessionProfile *Profile // 세션 시작 시 저장된 프로필

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
	e.hotkeyMgr.Register(input.KeyF9, e.stop)

	return e
}

// showSplash 스플래시 화면 표시
func (e *Engine) showSplash() {
	// 화면 지우기 (ANSI escape code)
	fmt.Print("\033[H\033[2J")

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("       🗡️  카카오톡 검키우기 매크로  🗡️")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("  만든이: 정지용")
	fmt.Println("  버그제보: hello@stopdragon.kr")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ⚠️  주의사항")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  본 소프트웨어는 학습 목적으로 제작되었습니다.")
	fmt.Println("  게임 내 자동화 도구 사용은 이용약관에 위배될 수")
	fmt.Println("  있으며, 계정 제재의 원인이 될 수 있습니다.")
	fmt.Println("  사용에 따른 모든 책임은 사용자에게 있으며,")
	fmt.Println("  제작자는 어떠한 책임도 지지 않습니다.")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// 5초 카운트다운
	for i := 5; i > 0; i-- {
		fmt.Printf("\r  %d초 후 프로그램이 시작됩니다... ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println()
}

// RunMenu 메인 메뉴 실행
func (e *Engine) RunMenu() {
	// 스플래시 화면 표시
	e.showSplash()

	reader := bufio.NewReader(os.Stdin)

	for {
		// 화면 지우기
		fmt.Print("\033[H\033[2J")

		fmt.Println()
		fmt.Println("========= 카카오톡 검키우기 =========")
		fmt.Println("만든이: 정지용 (hello@stopdragon.kr)")
		fmt.Println("=====================================")
		fmt.Println()
		fmt.Println("1. 강화 목표 달성")
		fmt.Println("2. 히든 아이템 뽑기")
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
			e.runHiddenMode(reader)
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
	if err != nil || target < 1 || target > 20 {
		fmt.Println("잘못된 레벨입니다. (1-20)")
		return
	}

	e.targetLevel = target
	e.mode = ModeEnhance
	e.setupAndRun()
}

func (e *Engine) runHiddenMode(reader *bufio.Reader) {
	fmt.Println()
	fmt.Println("=== 히든 아이템 뽑기 설정 ===")
	fmt.Println("히든 아이템을 찾으면 몇 레벨까지 강화할까요?")
	fmt.Println("(0 = 강화하지 않고 보관, 1-20 = 해당 레벨까지 강화)")
	fmt.Print("목표 레벨 (기본 0): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	targetLevel := 0
	if input != "" {
		if level, err := strconv.Atoi(input); err == nil && level >= 0 && level <= 20 {
			targetLevel = level
		}
	}

	e.targetLevel = targetLevel
	e.mode = ModeHidden
	e.setupAndRun()
}

func (e *Engine) runGoldMineMode() {
	// 서버 통계 기반 최적 레벨 조회
	optimalLevel, source := GetOptimalSellLevel(0)

	reader := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Println("=== 골드 채굴 설정 ===")
	fmt.Printf("📊 추천 판매 레벨: +%d (%s)\n", optimalLevel, source)
	fmt.Printf("⚙️  현재 설정값: +%d\n", e.cfg.GoldMineTarget)
	fmt.Println()
	fmt.Printf("목표 레벨 (엔터=%d): ", optimalLevel)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		e.targetLevel = optimalLevel
	} else if level, err := strconv.Atoi(input); err == nil && level >= 1 && level <= 20 {
		e.targetLevel = level
	} else {
		e.targetLevel = optimalLevel
	}

	fmt.Printf("✅ 목표 레벨: +%d\n", e.targetLevel)

	e.mode = ModeGoldMine
	e.setupAndRun()
}

func (e *Engine) runBattleMode(reader *bufio.Reader) {
	fmt.Println()
	fmt.Println("=== 자동 배틀 설정 ===")
	fmt.Printf("현재 역배 레벨 차이: %d (내 레벨 +1 ~ +%d 상대와 대결)\n",
		e.cfg.BattleLevelDiff, e.cfg.BattleLevelDiff)

	fmt.Print("역배 레벨 차이 (1-20, 엔터=유지): ")
	diffInput, _ := reader.ReadString('\n')
	diffInput = strings.TrimSpace(diffInput)
	if diff, err := strconv.Atoi(diffInput); err == nil && diff >= 1 && diff <= 20 {
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
		fmt.Println("카카오톡 메시지 입력창의 '메시지 입력' 글자에 마우스를 올려놓으세요...")
		fmt.Println("(입력창 안의 회색 플레이스홀더 텍스트)")
		fmt.Println("3초 후 자동으로 좌표를 저장합니다.")

		time.Sleep(3 * time.Second)

		e.cfg.ClickX, e.cfg.ClickY = input.GetMousePos()
		e.cfg.Save()

		fmt.Printf("좌표 저장됨: (%d, %d)\n", e.cfg.ClickX, e.cfg.ClickY)
	}

	// 입력창 위치 표시
	fmt.Println()
	fmt.Printf("📍 입력창 좌표: (%d, %d)\n", e.cfg.ClickX, e.cfg.ClickY)

	// 오버레이 표시 (채팅 영역, 입력 영역, 상태 패널, 컨트롤 버튼)
	overlay.ShowStatusOnly(e.cfg.ClickX, e.cfg.ClickY, e.cfg.ChatOffsetY,
		e.cfg.OverlayChatWidth, e.cfg.OverlayChatHeight,
		e.cfg.OverlayInputWidth, e.cfg.OverlayInputHeight)
	overlay.UpdateStatus("🎮 준비 중...\n카카오톡 창을 사이즈에 맞게 조정하세요")

	fmt.Println()
	fmt.Println("⚠️  카카오톡 채팅창을 사이즈에 맞게 조정하세요!")
	fmt.Println()

	// 5초 대기
	fmt.Print("⏳ 준비 대기: ")
	for i := 5; i > 0; i-- {
		fmt.Printf("%d... ", i)
		overlay.UpdateStatus("🎮 준비 중... %d초", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println()

	// 프로필 가져오기
	fmt.Println("📊 프로필 확인 중...")
	overlay.UpdateStatus("📊 프로필 확인 중...")
	e.sendCommand("/프로필")

	profileText := e.waitForResponse(10 * time.Second)
	if profileText == "" {
		fmt.Println("⚠️ 프로필을 가져올 수 없습니다. 계속 진행합니다.")
	} else {
		e.sessionProfile = ParseProfile(profileText)
		if e.sessionProfile != nil && e.sessionProfile.Name != "" {
			fmt.Printf("✅ 프로필 확인: %s\n", e.sessionProfile.Name)
			fmt.Printf("   보유 검: [+%d] %s\n", e.sessionProfile.Level, e.sessionProfile.SwordName)
			fmt.Printf("   보유 골드: %sG\n", FormatGold(e.sessionProfile.Gold))

			// 텔레메트리에 프로필 정보 전송
			e.telem.RecordProfile(e.sessionProfile.Name, e.sessionProfile.Level, e.sessionProfile.Gold)
		}
	}

	fmt.Println()
	fmt.Println("🚀 시작!")
	overlay.UpdateStatus("🚀 시작!")

	// 핫키 시작
	e.hotkeyMgr.Start()
	defer e.hotkeyMgr.Stop()

	fmt.Println()
	fmt.Println("=== 매크로 시작 ===")
	fmt.Println("F8: 일시정지/재개")
	fmt.Println("F9: 종료")
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
		delay := e.getDelayForLevel(state.Level)
		time.Sleep(delay)

		// 결과 확인 및 골드 부족 체크
		text := e.readChatText()
		if text != "" {
			goldInfo := DetectInsufficientGold(text)
			if goldInfo.IsInsufficient {
				e.handleInsufficientGold(goldInfo)
				return
			}
		}
	}
}

func (e *Engine) loopHidden() {
	// 초기 상태 표시
	targetStr := "보관"
	if e.targetLevel > 0 {
		targetStr = fmt.Sprintf("+%d까지 강화", e.targetLevel)
	}
	overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n목표: %s\n트래시: 0회", targetStr)

	retryCount := 0
	const maxRetries = 3

	for e.running {
		if e.checkStop() {
			return
		}

		// 1. /판매 시도 (현재 검 팔고 새 검 받기)
		overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n트래시: %d회\n📤 /판매 전송...", e.sessionStats.trashCount)
		e.sendCommand("/판매")

		// 응답 대기
		overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n트래시: %d회\n⏳ 응답 대기...", e.sessionStats.trashCount)

		// 결과 확인 (응답 변경 감지 + 재시도 로직)
		var text string
		var state *GameState
		readSuccess := false

		for retry := 0; retry < maxRetries && !readSuccess; retry++ {
			if retry > 0 {
				fmt.Printf("  🔄 재시도 %d/%d...\n", retry+1, maxRetries)
				overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n🔄 재시도 %d/%d", retry+1, maxRetries)
			}

			overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n트래시: %d회\n🔍 채팅창 분석...", e.sessionStats.trashCount)
			// 응답이 변경될 때까지 대기 (최대 5초)
			text = e.readChatTextWaitForChange(5 * time.Second)

			// 텍스트가 비어있으면 재시도
			if text == "" {
				continue
			}

			// 판매 불가 체크 (0강 아이템) - 이 경우만 /강화 허용
			if CannotSell(text) {
				overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n트래시: %d회\n⚔️ 0강 → 강화 파괴", e.sessionStats.trashCount)
				e.sendCommand("/강화")
				time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
				readSuccess = true // 처리 완료
				break
			}

			state = ParseOCRText(text)
			if state != nil {
				readSuccess = true
				retryCount = 0 // 성공하면 리셋
			}
		}

		// 계속 실패하면 사용자에게 알림 (아이템 파괴하지 않음!)
		if !readSuccess {
			retryCount++
			fmt.Printf("  ⚠️ 채팅창 읽기 %d회 연속 실패 - 아이템 보존\n", retryCount)
			overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n⚠️ 읽기 실패 %d회\n채팅창 확인!", retryCount)

			if retryCount >= 5 {
				fmt.Println("\n❌ 채팅창 읽기가 계속 실패합니다!")
				fmt.Println("📋 확인 사항:")
				fmt.Println("   1. 카카오톡 창이 활성화되어 있는지 확인")
				fmt.Println("   2. 입력창 좌표가 정확한지 확인")
				fmt.Println("\n⏸️ 3초 후 재시도합니다...")
				time.Sleep(3 * time.Second)
				retryCount = 0
			} else {
				time.Sleep(1 * time.Second)
			}
			continue
		}

		// state가 nil이면 다음 루프
		if state == nil {
			continue
		}

		// 아이템 이름 추출
		itemName := state.ItemName
		if itemName == "" {
			itemName = ExtractItemName(text)
		}

		// 디버그: 아이템 타입 출력
		fmt.Printf("  📋 감지: [%s] %s\n", state.ItemType, itemName)

		// 2. 히든이면 성공
		if state.ItemType == "hidden" {
			overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n🎉 히든 발견!\n[%s]\n\n📋 판단: 히든 → 보관/강화", itemName)
			fmt.Printf("\n🎉 히든 아이템 발견! [%s]\n", itemName)
			logger.Info("히든 아이템 발견: %s", itemName)

			// 텔레메트리: 아이템 이름 포함
			e.telem.RecordFarmingWithItem(itemName, "hidden")
			e.telem.RecordSword()
			e.sessionStats.hiddenCount++

			// 강화 목표가 있으면 강화 진행
			if e.targetLevel > 0 {
				fmt.Printf("📈 목표 +%d까지 강화를 시작합니다...\n", e.targetLevel)
				overlay.UpdateStatus("⭐ 히든 강화 중\n[%s]\n목표: +%d", itemName, e.targetLevel)

				// 골드 체크
				if e.sessionProfile != nil && e.sessionProfile.Gold < 1000 {
					fmt.Println("⚠️ 골드가 부족하여 강화를 진행할 수 없습니다.")
					e.telem.TrySend()
					return
				}

				// 강화 진행
				finalLevel, success := e.enhanceToTargetWithLevel(itemName)
				if success {
					fmt.Printf("✅ 강화 완료! [%s] +%d\n", itemName, finalLevel)
					overlay.UpdateStatus("⭐ 히든 강화 완료!\n[%s] +%d", itemName, finalLevel)
				} else {
					fmt.Printf("💥 강화 중 파괴됨 (최종 레벨: +%d)\n", finalLevel)
					overlay.UpdateStatus("💥 히든 파괴됨\n[%s] +%d", itemName, finalLevel)
				}
			}

			e.telem.TrySend()
			return
		}

		// 3. 트래시/일반이면 /강화로 파괴
		if state.ItemType == "trash" || state.ItemType == "normal" || state.ItemType == "unknown" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			displayName := itemName
			if displayName == "" {
				displayName = state.ItemType
			}
			overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n트래시: %d회\n🗑️ %s\n\n📋 판단: %s → 파괴", e.sessionStats.trashCount, displayName, state.ItemType)
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// 4. 알 수 없는 타입 - 안전하게 처리
		fmt.Printf("  ❓ 알 수 없는 아이템 타입: [%s] - 다음 사이클 진행\n", state.ItemType)
		overlay.UpdateStatus("⭐ 히든 아이템 뽑기\n❓ 타입 불명\n다음 사이클...")
		time.Sleep(500 * time.Millisecond)
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
		finalLevel, success := e.enhanceToTargetWithLevel(itemName)
		if !success {
			e.telem.RecordCycle(false)
			continue
		}

		// 3. 판매
		overlay.UpdateStatus("💰 골드 채굴 #%d\n💵 판매 중: %s +%d\n누적: %sG\n\n📋 판단: +%d 달성 → 판매", e.cycleCount, itemName, finalLevel, FormatGold(e.totalGold), e.targetLevel)
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
		levelDiff := target.Level - e.myProfile.Level
		fmt.Printf("⚔️ #%d: %s (+%d) vs 나 (+%d) [%s]\n",
			e.cycleCount, target.Username, target.Level, e.myProfile.Level, e.myProfile.SwordName)
		overlay.UpdateStatus("⚔️ 자동 배틀 #%d\n타겟: %s +%d\n내 레벨: +%d\n\n📋 판단: +%d차 역배 도전", e.cycleCount, target.Username, target.Level, e.myProfile.Level, levelDiff)

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
			overlay.UpdateStatus("⚔️ 자동 배틀\n🏆 승리! +%sG\n전적: %d승 %d패\n\n📋 판단: 역배 성공", FormatGold(goldEarned), e.battleWins, e.battleLosses)
		} else {
			e.battleLosses++
			fmt.Println("   → 💔 패배...")
			overlay.UpdateStatus("⚔️ 자동 배틀\n💔 패배...\n전적: %d승 %d패\n\n📋 판단: 역배 실패", e.battleWins, e.battleLosses)
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

// 이전 텍스트 결과 저장 (응답 대기용)
var lastChatText string

// readChatText 화면에서 텍스트 읽기 (클립보드 방식)
// 내 메시지만 필터링하여 반환 (다른 사람 메시지 무시)
func (e *Engine) readChatText() string {
	text := e.readChatClipboard()
	// 내 메시지만 필터링 (프로필이 있는 경우)
	return e.filterMyMessages(text)
}

// readChatClipboard 클립보드 복사 방식으로 채팅 텍스트 읽기
func (e *Engine) readChatClipboard() string {
	// 입력창 좌표 (명령어 입력용)
	inputX := e.cfg.ClickX
	inputY := e.cfg.ClickY

	// 채팅 영역 왼쪽 하단에서 25x25 위치 클릭
	// 채팅 영역 왼쪽 = clickX - 20
	// 채팅 영역 하단 = clickY - 20 - 2 (입력 영역 상단에서 2픽셀 위)
	chatClickX := e.cfg.ClickX - 20 + 25  // 채팅 영역 왼쪽에서 25px 오른쪽
	chatClickY := e.cfg.ClickY - 22 - 25  // 채팅 영역 하단에서 25px 위

	// 채팅 영역에서 텍스트 읽기 (전체선택 → 복사 → 클립보드)
	text := input.ReadChatText(chatClickX, chatClickY, inputX, inputY)

	if text == "" {
		fmt.Println("  ⚠️ 클립보드 텍스트 비어있음")
	}

	logger.OCR(text) // 로그는 동일한 형식 유지
	return text
}

// readOCRText 하위 호환 (기존 함수명 유지)
func (e *Engine) readOCRText() string {
	return e.readChatText()
}

// readChatTextWaitForChange 응답이 올 때까지 대기하며 텍스트 읽기
// 이전 결과와 다를 때까지 최대 maxWait 동안 대기
func (e *Engine) readChatTextWaitForChange(maxWait time.Duration) string {
	startTime := time.Now()
	pollInterval := 300 * time.Millisecond

	for time.Since(startTime) < maxWait {
		text := e.readChatText()
		if text == "" {
			time.Sleep(pollInterval)
			continue
		}

		// 이전 결과와 다르면 (새 응답 도착) 반환
		if !isSameTextResult(text, lastChatText) {
			lastChatText = text
			return text
		}

		// 같으면 대기 후 재시도
		time.Sleep(pollInterval)
	}

	// 타임아웃 - 마지막으로 읽은 결과 반환
	text := e.readChatText()
	if text != "" {
		lastChatText = text
	}
	return text
}

// readOCRTextWaitForChange 하위 호환
func (e *Engine) readOCRTextWaitForChange(maxWait time.Duration) string {
	return e.readChatTextWaitForChange(maxWait)
}

// waitForResponse 플레이봇 응답 대기 (최대 maxWait 동안)
// 명령어 전송 후 응답이 올 때까지 대기
func (e *Engine) waitForResponse(maxWait time.Duration) string {
	startTime := time.Now()
	pollInterval := 500 * time.Millisecond
	initialWait := 1 * time.Second

	// 최소 대기 (명령어 처리 시간)
	time.Sleep(initialWait)

	for time.Since(startTime) < maxWait {
		text := e.readChatText()
		if text == "" {
			time.Sleep(pollInterval)
			continue
		}

		// 이전 결과와 다르면 새 응답
		if !isSameTextResult(text, lastChatText) {
			lastChatText = text
			return text
		}

		time.Sleep(pollInterval)
	}

	return ""
}

// filterMyMessages 내 메시지만 필터링 (가장 최근 @이름 섹션만)
func (e *Engine) filterMyMessages(text string) string {
	if e.sessionProfile == nil || e.sessionProfile.Name == "" {
		return text // 프로필 없으면 전체 반환
	}

	myName := e.sessionProfile.Name
	lines := strings.Split(text, "\n")

	// 가장 마지막 내 메시지 섹션의 시작점 찾기
	lastMyIndex := -1
	for i, line := range lines {
		if strings.Contains(line, "@") {
			if strings.Contains(line, myName) {
				lastMyIndex = i // 마지막 내 섹션 시작점 갱신
			}
		}
	}

	// 내 섹션이 없으면 전체 반환
	if lastMyIndex == -1 {
		return text
	}

	// 마지막 내 섹션부터 끝까지 또는 다른 사람 섹션 시작 전까지
	var result []string
	for i := lastMyIndex; i < len(lines); i++ {
		line := lines[i]

		// 다른 사람의 섹션이 시작되면 중단
		if i > lastMyIndex && strings.Contains(line, "@") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "@") && !strings.Contains(line, myName) {
				break
			}
		}

		result = append(result, line)
	}

	if len(result) == 0 {
		return text
	}

	return strings.Join(result, "\n")
}

// isSameTextResult 텍스트 결과가 동일한지 비교 (diff 기반)
// 이전 텍스트의 끝부분이 현재 텍스트에 포함되어 있고, 그 뒤에 새 텍스트가 없으면 "같음"
func isSameTextResult(current, previous string) bool {
	if previous == "" {
		return false // 이전 결과 없으면 항상 다른 것으로 처리
	}
	if current == "" {
		return true // 현재 결과가 비어있으면 같은 것으로 처리
	}

	// 이전 텍스트의 마지막 부분 (비교용 키)
	// 너무 짧으면 오탐 가능, 너무 길면 못 찾을 수 있음
	keyLen := 100
	if len(previous) < keyLen {
		keyLen = len(previous)
	}
	key := previous[len(previous)-keyLen:]

	// 현재 텍스트에서 키가 어디에 있는지 찾기
	idx := strings.LastIndex(current, key)
	if idx == -1 {
		// 키를 못 찾으면 완전히 다른 텍스트 → 새 응답
		return false
	}

	// 키 이후에 새로운 텍스트가 있는지 확인
	afterKey := current[idx+len(key):]
	newText := strings.TrimSpace(afterKey)

	// 새 텍스트가 없으면 같은 결과 (새 응답 없음)
	return len(newText) == 0
}

func (e *Engine) farmUntilHidden() bool {
	_, found := e.farmUntilHiddenWithName()
	return found
}

// farmUntilHiddenWithName 히든 아이템을 찾을 때까지 파밍하고 아이템 이름 반환
// 로직: /판매 → 채팅창 읽기 → 트래시면 /강화(파괴) → 반복, 히든이면 반환
func (e *Engine) farmUntilHiddenWithName() (string, bool) {
	retryCount := 0
	const maxRetries = 3

	for e.running {
		if e.checkStop() {
			return "", false
		}

		// 1. /판매 시도 (현재 검 팔고 새 검 받기)
		e.sendCommand("/판매")

		// 결과 확인 (응답 변경 감지 + 재시도 로직)
		var text string
		var state *GameState
		readSuccess := false

		for retry := 0; retry < maxRetries && !readSuccess; retry++ {
			if retry > 0 {
				fmt.Printf("  🔄 재시도 %d/%d...\n", retry+1, maxRetries)
			}

			// 응답이 변경될 때까지 대기 (최대 5초)
			text = e.readChatTextWaitForChange(5 * time.Second)

			// 텍스트가 비어있으면 재시도
			if text == "" {
				continue
			}

			// 2. 판매 불가 체크 (0강 아이템은 판매 불가) - 이 경우만 /강화 허용
			if CannotSell(text) {
				// 0강 아이템은 /강화로 파괴
				e.sendCommand("/강화")
				time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
				readSuccess = true
				break
			}

			// 3. 새 검 획득 체크
			state = ParseOCRText(text)
			if state != nil {
				readSuccess = true
				retryCount = 0
			}
		}

		// 계속 실패하면 경고 (아이템 파괴하지 않음!)
		if !readSuccess {
			retryCount++
			fmt.Printf("  ⚠️ 채팅창 읽기 %d회 연속 실패 - 아이템 보존\n", retryCount)

			if retryCount >= 5 {
				fmt.Println("\n❌ 채팅창 읽기가 계속 실패합니다! 카카오톡 창 상태를 확인하세요.")
				time.Sleep(3 * time.Second)
				retryCount = 0
			} else {
				time.Sleep(1 * time.Second)
			}
			continue
		}

		// state가 nil이면 (0강 처리로 이미 continue된 경우) 다음 루프
		if state == nil {
			continue
		}

		// 아이템 이름 추출
		itemName := state.ItemName
		if itemName == "" {
			itemName = ExtractItemName(text)
		}

		// 4. 히든 아이템이면 반환 (강화 모드로 전환)
		if state.ItemType == "hidden" {
			e.telem.RecordFarmingWithItem(itemName, "hidden")
			e.sessionStats.hiddenCount++
			fmt.Printf("🎉 히든 발견! [%s]\n", itemName)
			overlay.UpdateStatus("💰 골드 채굴 #%d\n🎉 히든 발견!\n[%s]\n\n📋 판단: 히든 → 강화", e.cycleCount, itemName)
			return itemName, true
		}

		// 5. 트래시/일반 아이템이면 /강화로 파괴하고 반복
		if state.ItemType == "trash" || state.ItemType == "normal" || state.ItemType == "unknown" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			displayName := itemName
			if displayName == "" {
				displayName = state.ItemType
			}
			overlay.UpdateStatus("💰 골드 채굴 #%d\n🗑️ %s\n\n📋 판단: %s → 파괴\n트래시: %d회", e.cycleCount, displayName, state.ItemType, e.sessionStats.trashCount)
			// 트래시는 /강화로 파괴 (0강이므로 바로 파괴됨)
			e.sendCommand("/강화")
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// 6. 알 수 없는 타입이면 다음 사이클
		fmt.Printf("  ❓ 알 수 없는 타입: [%s]\n", state.ItemType)
		time.Sleep(500 * time.Millisecond)
	}
	return "", false
}

func (e *Engine) enhanceToTarget() bool {
	_, success := e.enhanceToTargetWithLevel("")
	return success
}

// enhanceToTargetWithLevel 목표까지 강화하고 최종 레벨 반환
func (e *Engine) enhanceToTargetWithLevel(swordName string) (int, bool) {
	currentLevel := 0

	for currentLevel < e.targetLevel && e.running {
		if e.checkStop() {
			return currentLevel, false
		}

		e.sendCommand("/강화")
		delay := e.getDelayForLevel(currentLevel)
		time.Sleep(delay)

		// 채팅 텍스트 읽기
		text := e.readChatText()
		if text == "" {
			continue
		}

		// 골드 부족 감지
		goldInfo := DetectInsufficientGold(text)
		if goldInfo.IsInsufficient {
			e.handleInsufficientGold(goldInfo)
			return currentLevel, false
		}

		state := ParseOCRText(text)
		if state == nil {
			continue
		}

		switch state.LastResult {
		case "success":
			currentLevel++
			fmt.Printf("  ✅ +%d 성공\n", currentLevel)
			e.telem.RecordEnhanceWithSword(swordName, currentLevel-1, "success")
			e.sessionStats.enhanceSuccess++
			overlay.UpdateStatus("⚔️ 강화 중\n+%d/%d\n\n📋 판단: 성공 → 계속", currentLevel, e.targetLevel)
		case "destroy":
			fmt.Println("  💥 파괴!")
			e.telem.RecordEnhanceWithSword(swordName, currentLevel, "destroy")
			e.sessionStats.enhanceDestroy++
			overlay.UpdateStatus("⚔️ 강화 중\n💥 파괴!\n\n📋 판단: 파괴 → 새 아이템")
			return currentLevel, false
		case "hold":
			fmt.Printf("  ⏸️ +%d 유지\n", currentLevel)
			e.telem.RecordEnhanceWithSword(swordName, currentLevel, "hold")
			e.sessionStats.enhanceHold++
			overlay.UpdateStatus("⚔️ 강화 중\n+%d/%d\n\n📋 판단: 유지 → 재시도", currentLevel, e.targetLevel)
		}
	}

	return currentLevel, currentLevel >= e.targetLevel
}

// handleInsufficientGold 골드 부족 시 종료 절차 수행
func (e *Engine) handleInsufficientGold(info *InsufficientGoldInfo) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  💸 골드 부족으로 종료합니다!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if info.RequiredGold > 0 {
		fmt.Printf("  💰 필요 골드: %sG\n", FormatGold(info.RequiredGold))
	}
	if info.RemainingGold >= 0 {
		fmt.Printf("  💵 남은 골드: %sG\n", FormatGold(info.RemainingGold))
	}
	if info.RequiredGold > 0 && info.RemainingGold >= 0 {
		shortage := info.RequiredGold - info.RemainingGold
		fmt.Printf("  📉 부족 골드: %sG\n", FormatGold(shortage))
	}

	fmt.Println()
	fmt.Println("  💡 골드를 더 모은 후 다시 시도하세요!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// 오버레이 업데이트
	overlay.UpdateStatus("💸 골드 부족!\n필요: %sG\n남은: %sG",
		FormatGold(info.RequiredGold), FormatGold(info.RemainingGold))

	// 텔레메트리 전송
	fmt.Println("📤 통계 전송 중...")
	e.telem.Flush()
	fmt.Println("✅ 전송 완료!")

	// 실행 중지
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
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
	// 클립보드 방식으로 텍스트 읽기
	text := e.readChatText()
	if text == "" {
		return nil
	}
	return ParseOCRText(text)
}

func (e *Engine) readCurrentGold() int {
	text := e.readChatText()
	if text == "" {
		return 0
	}
	state := ParseOCRText(text)
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
	// 오버레이 버튼 클릭 체크
	if overlay.CheckStopClicked() {
		fmt.Println("\n⏹️ 종료 버튼 클릭!")
		e.running = false
		return true
	}
	if overlay.CheckRestartClicked() {
		fmt.Println("\n🔄 재시작 버튼 클릭!")
		e.running = false
		return true
	}
	if overlay.CheckPauseClicked() {
		e.togglePause()
	}

	// 일시정지 체크
	for e.paused && e.running {
		overlay.UpdateStatus("⏸️ 일시정지\nF8 또는 버튼 클릭으로 재개")
		// 일시정지 중에도 버튼 체크
		if overlay.CheckPauseClicked() {
			e.togglePause()
			break
		}
		if overlay.CheckStopClicked() {
			fmt.Println("\n⏹️ 종료 버튼 클릭!")
			e.running = false
			return true
		}
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

func (e *Engine) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	fmt.Println("\n⏹️ F9 종료!")
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
			fmt.Print("감속 시작 레벨 (1-20): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 1 && v <= 20 {
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
			fmt.Print("골드 채굴 목표 레벨 (1-20): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 1 && v <= 20 {
				e.cfg.GoldMineTarget = v
			}
		case "6":
			fmt.Print("배틀 역배 레벨차 (1-20): ")
			val, _ := reader.ReadString('\n')
			if v, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && v >= 1 && v <= 20 {
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
		fmt.Println("카카오톡 메시지 입력창의 '메시지 입력' 글자에 마우스를 올려놓으세요...")
		fmt.Println("(입력창 안의 회색 플레이스홀더 텍스트)")
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

	// 오버레이 표시 (채팅 영역, 입력 영역, 상태 패널)
	overlay.ShowStatusOnly(e.cfg.ClickX, e.cfg.ClickY, e.cfg.ChatOffsetY,
		e.cfg.OverlayChatWidth, e.cfg.OverlayChatHeight,
		e.cfg.OverlayInputWidth, e.cfg.OverlayInputHeight)
	overlay.UpdateStatus("🔍 프로필 분석 중...")

	// 3초 대기
	fmt.Print("⏳ 준비 대기: ")
	for i := 3; i > 0; i-- {
		fmt.Printf("%d... ", i)
		overlay.UpdateStatus("🔍 프로필 분석\n%d초 후 시작...", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("시작!")
	fmt.Println()

	// /프로필 명령어 전송
	fmt.Println("📤 /프로필 명령어 전송 중...")
	overlay.UpdateStatus("📤 /프로필 전송 중...")
	e.sendCommand("/프로필")
	fmt.Println("⏳ 응답 대기 중 (2초)...")
	time.Sleep(2 * time.Second)

	// 클립보드로 프로필 읽기
	fmt.Println("🔍 채팅 텍스트 읽는 중...")
	profileText := e.readChatText()

	// 디버그: 결과 출력
	if profileText == "" {
		fmt.Println("⚠️ 텍스트를 읽을 수 없습니다.")
		fmt.Println()
		fmt.Println("🔧 문제 해결 방법:")
		fmt.Println("   1. 카카오톡 창이 활성화되어 있는지 확인")
		fmt.Println("   2. 메시지 입력창 위치가 맞는지 확인")
		fmt.Println("   3. 좌표 고정 해제 후 다시 시도 (옵션 설정 → 좌표 고정)")
		overlay.HideAll()
		return
	}

	profile := ParseProfile(profileText)

	if profile == nil || profile.Level < 0 {
		fmt.Println("❌ 프로필을 파싱할 수 없습니다.")
		fmt.Println()
		fmt.Println("📝 읽은 텍스트 (처음 200자):")
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
		overlay.HideAll()
		return
	}

	// 프로필 분석 완료 - 상태 패널에 요약 표시
	overlay.UpdateStatus("📊 프로필 분석 완료\n\n%s\n[+%d] %s\n💰 %sG\n\n0번: 돌아가기",
		profile.Name, profile.Level, profile.SwordName, FormatGold(profile.Gold))

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

	// 현재 레벨부터 +20까지 표시
	rates := GetAllEnhanceRates()
	for lvl := profile.Level; lvl <= 20 && rates != nil && lvl < len(rates); lvl++ {
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
	targets := []int{profile.Level + 1, profile.Level + 2, profile.Level + 3, 10, 12, 15, 20}
	shown := make(map[int]bool)

	for _, target := range targets {
		if target <= profile.Level || target > 20 || shown[target] {
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
	fmt.Println()
	fmt.Print("0. 돌아가기\n선택: ")

	// 사용자 입력 대기
	reader := bufio.NewReader(os.Stdin)
	for {
		userInput, _ := reader.ReadString('\n')
		userInput = strings.TrimSpace(userInput)
		if userInput == "0" {
			break
		}
		fmt.Print("0을 입력하여 돌아가세요: ")
	}

	// 오버레이 숨기기
	overlay.HideAll()
}
