package game

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
	ModeSpecial  // 특수 아이템 뽑기
	ModeGoldMine // 골드 채굴
	ModeBattle   // 자동 배틀 (역배)
)

// Engine 게임 엔진
type Engine struct {
	cfg       *config.Config
	telem     *telemetry.Telemetry
	mode      Mode
	running bool
	mu      sync.Mutex

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

	// 세션 프로필 (필터링용)
	sessionProfile *Profile // 세션 시작 시 저장된 프로필

	// 이전 RAW 텍스트 (응답 변경 감지용)
	lastRawChatText string

	// 세션 통계 (종료 시 출력용)
	sessionStats struct {
		startGold       int
		endGold         int
		trashCount    int // 쓰레기 처리 횟수
		specialCount  int // 특수 아이템 발견 횟수
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
		fmt.Println("2. 특수 아이템 뽑기")
		fmt.Println("3. 골드 채굴 (돈벌기)")
		fmt.Println("4. 자동 배틀 (역배)")
		fmt.Println("5. 내 프로필 분석")
		fmt.Println("6. 옵션 설정")
		fmt.Println("0. 종료")
		fmt.Println()
		fmt.Print("선택: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			e.runEnhanceMode(reader)
		case "2":
			e.runSpecialMode(reader)
		case "3":
			e.runGoldMineMode()
		case "4":
			e.runBattleMode(reader)
		case "5":
			e.showMyProfile()
		case "6":
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
	if err != nil || target < 1 || target > 20 {
		fmt.Println("잘못된 레벨입니다. (1-20)")
		return
	}

	e.targetLevel = target
	e.mode = ModeEnhance
	e.setupAndRun()
}

func (e *Engine) runSpecialMode(reader *bufio.Reader) {
	fmt.Println()
	fmt.Println("=== 특수 아이템 뽑기 설정 ===")
	fmt.Println("특수 아이템을 찾으면 몇 레벨까지 강화할까요?")
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
	e.mode = ModeSpecial
	e.setupAndRun()
}

func (e *Engine) runGoldMineMode() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Println("=== 골드 채굴 설정 ===")

	// 서버 통계 기반 최적 레벨 조회 (로딩 표시)
	fmt.Print("📊 서버 데이터 분석 중...")
	optimalLevel, source := GetOptimalSellLevel(0)
	efficiencies := GetAllLevelEfficiencies()
	fmt.Print("\r                              \r") // 로딩 메시지 지우기

	fmt.Printf("📊 추천 판매 레벨: +%d (%s)\n", optimalLevel, source)
	fmt.Printf("⚙️  현재 설정값: +%d\n", e.cfg.GoldMineTarget)

	// 레벨별 효율성 표시 (서버 데이터가 있는 경우)
	if len(efficiencies) > 0 {
		fmt.Println()
		fmt.Println("📈 레벨별 시간 효율 (G/분):")
		fmt.Println("   레벨 |  판매가  | 성공률 | G/분")
		fmt.Println("   -----|---------|--------|-------")
		for _, eff := range efficiencies {
			marker := "  "
			if eff.Recommendation == "optimal" {
				marker = "★ "
			}
			fmt.Printf("   %s+%2d | %7s | %5.1f%% | %s\n",
				marker,
				eff.Level,
				FormatGold(eff.AvgPrice),
				eff.SuccessProb,
				FormatGold(int(eff.GoldPerMinute)),
			)
		}
		fmt.Println("   (★ = 최적 레벨)")
	}

	fmt.Println()
	// 최적 레벨(★)을 기본값으로 사용 (시간 효율 최대화)
	defaultTarget := optimalLevel
	fmt.Printf("목표 레벨 (엔터=%d): ", defaultTarget)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		e.targetLevel = defaultTarget
	} else if level, err := strconv.Atoi(input); err == nil && level >= 1 && level <= 20 {
		e.targetLevel = level
	} else {
		e.targetLevel = defaultTarget
	}

	// 선택한 레벨의 효율성 정보 표시
	if eff := GetLevelEfficiency(e.targetLevel); eff != nil {
		fmt.Printf("✅ 목표 레벨: +%d (예상 %.0f G/분, 성공률 %.1f%%)\n",
			e.targetLevel, eff.GoldPerMinute, eff.SuccessProb)
	} else {
		fmt.Printf("✅ 목표 레벨: +%d\n", e.targetLevel)
	}

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
	// 카카오톡 포커스 확보 (카운트다운 중 터미널에 포커스 있을 수 있음)
	input.Click(e.cfg.ClickX, e.cfg.ClickY)
	time.Sleep(300 * time.Millisecond)
	e.SaveLastChatText()
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
	fmt.Println("F9: 종료")
	fmt.Println()

	e.running = true
	e.cycleCount = 0
	e.totalGold = 0
	e.startTime = time.Now()

	// 세션 통계 초기화
	e.sessionStats.startGold = e.readCurrentGold()
	e.sessionStats.endGold = 0
	e.sessionStats.trashCount = 0
	e.sessionStats.specialCount = 0
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

	// 채팅 상태 초기화 (첫 로그에 전체 이력 방지)
	// RAW 텍스트 저장 (변경 감지 기준점)
	initialText := e.readChatClipboard()
	if initialText != "" {
		e.lastRawChatText = initialText
	}

	// 텔레메트리에 모드 설정 (v3)
	switch e.mode {
	case ModeEnhance:
		e.telem.SetMode("enhance")
	case ModeSpecial:
		e.telem.SetMode("special")
	case ModeGoldMine:
		e.telem.SetMode("goldmine")
	case ModeBattle:
		e.telem.SetMode("battle")
	}

	// 모드별 실행
	switch e.mode {
	case ModeEnhance:
		e.loopEnhance()
	case ModeSpecial:
		e.loopSpecial()
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
	if e.sessionStats.trashCount > 0 || e.sessionStats.specialCount > 0 {
		fmt.Printf("  🗑️ 쓰레기 처리: %d회\n", e.sessionStats.trashCount)
		fmt.Printf("  ⭐ 특수 발견:   %d회\n", e.sessionStats.specialCount)
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
	// 시작 시 프로필 정보 표시 (Run()에서 이미 조회한 sessionProfile 사용)
	// 중복 /프로필 전송 방지
	overlay.UpdateStatus("⚔️ 강화 모드\n목표: +%d", e.targetLevel)

	if e.sessionProfile != nil && e.sessionProfile.SwordName != "" {
		fmt.Printf("📋 현재 보유 검: [+%d] %s\n", e.sessionProfile.Level, e.sessionProfile.SwordName)

		// 이미 목표 달성한 경우 종료
		if e.IsTargetReached(e.sessionProfile.Level) {
			fmt.Printf("\n✅ 이미 목표 달성! 현재 +%d (목표: +%d)\n", e.sessionProfile.Level, e.targetLevel)
			fmt.Println("💡 강화할 필요가 없습니다. 메뉴로 돌아갑니다.")
			overlay.UpdateStatus("⚔️ 강화 불필요\n✅ 이미 +%d 보유!\n목표: +%d\n\n📋 판단: 목표 이미 달성", e.sessionProfile.Level, e.targetLevel)
			time.Sleep(2 * time.Second)
			return
		}

		// 현재 레벨이 0보다 크면 기존 검으로 계속 강화
		if e.sessionProfile.Level > 0 {
			fmt.Printf("📈 현재 +%d에서 목표 +%d까지 강화를 시작합니다.\n", e.sessionProfile.Level, e.targetLevel)
			overlay.UpdateStatus("⚔️ 강화 모드\n현재: +%d → 목표: +%d\n[%s]\n\n📋 판단: 기존 검 강화 계속", e.sessionProfile.Level, e.targetLevel, e.sessionProfile.SwordName)
		} else {
			fmt.Printf("📈 +0에서 목표 +%d까지 강화를 시작합니다.\n", e.targetLevel)
			overlay.UpdateStatus("⚔️ 강화 모드\n현재: +0 → 목표: +%d\n\n📋 판단: 새 검 강화 시작", e.targetLevel)
		}
	}

	fmt.Println()

	// 시작 레벨/검 이름 초기화 (sessionProfile에서, readGameState 아님)
	currentLevel := 0
	swordName := ""
	if e.sessionProfile != nil {
		currentLevel = e.sessionProfile.Level
		swordName = e.sessionProfile.SwordName
	}

	// 변경 감지 기준점 초기화
	e.ResetLastChatText()

	for e.running {
		if e.checkStop() {
			return
		}

		// 목표 달성 확인
		if e.IsTargetReached(currentLevel) {
			fmt.Printf("\n🎉 목표 달성! +%d\n", currentLevel)
			logger.Info("목표 달성: +%d", currentLevel)
			overlay.UpdateStatus("⚔️ 강화 완료!\n🎉 +%d 달성!\n\n📋 판단: 목표 도달 → 완료", currentLevel)
			e.ReportSwordComplete()
			return
		}

		// 강화 명령
		overlay.UpdateStatus("⚔️ 강화 중\n현재: +%d → 목표: +%d\n\n📋 판단: /강화 실행", currentLevel, e.targetLevel)
		e.sendCommand("/강화")
		delay := e.getDelayForLevel(currentLevel)
		time.Sleep(delay)

		// 결과 확인 - 게임 응답이 올 때까지 대기
		text := e.readChatTextWaitForChange(5 * time.Second)

		// 응답이 없으면 재시도 (게임 응답 전에 읽은 경우)
		if text == "" {
			for retry := 0; retry < 3 && e.running; retry++ {
				time.Sleep(1 * time.Second)
				text = e.readChatTextWaitForChange(3 * time.Second)
				if text != "" {
					break
				}
			}
		}

		if text == "" {
			continue
		}

		// 골드 부족 체크
		goldInfo := DetectInsufficientGold(text)
		if goldInfo.IsInsufficient {
			overlay.UpdateStatus("⚔️ 강화 중단\n💰 골드 부족!\n필요: %s\n보유: %s",
				FormatGold(goldInfo.RequiredGold), FormatGold(goldInfo.RemainingGold))
			e.handleInsufficientGold(goldInfo)
			return
		}

		// 강화 결과 파싱 + 상태 추적
		state := ParseOCRText(text)
		if state == nil {
			continue
		}

		switch state.LastResult {
		case "destroy":
			e.sessionStats.enhanceDestroy++
			e.telem.RecordEnhanceWithSword(swordName, currentLevel, "destroy")
			fmt.Printf("  💥 +%d에서 파괴!\n", currentLevel)
			overlay.UpdateStatus("⚔️ 강화 중\n💥 +%d 파괴!\n\n📋 판단: 새 검으로 재시작", currentLevel)

			// 새 검 정보 추출
			if name, _, found := ExtractDestroyNewSword(text); found {
				swordName = name
			} else {
				swordName = "낡은 검"
			}
			currentLevel = 0

		case "success":
			e.sessionStats.enhanceSuccess++
			if state.ResultLevel > 0 {
				currentLevel = state.ResultLevel
			} else {
				currentLevel++
			}
			e.telem.RecordEnhanceWithSword(swordName, currentLevel-1, "success")
			fmt.Printf("  ⚔️ 강화 성공! +%d\n", currentLevel)
			overlay.UpdateStatus("⚔️ 강화 중\n현재: +%d → 목표: +%d\n\n📋 판단: 성공!", currentLevel, e.targetLevel)

		case "hold":
			e.sessionStats.enhanceHold++
			if state.ResultLevel > 0 && state.ResultLevel != currentLevel {
				currentLevel = state.ResultLevel
			}
			e.telem.RecordEnhanceWithSword(swordName, currentLevel, "hold")
			fmt.Printf("  💫 +%d 유지\n", currentLevel)

		default:
			// 결과 불명확 — ResultLevel로 동기화 시도
			if state.ResultLevel > 0 && state.ResultLevel != currentLevel {
				currentLevel = state.ResultLevel
			}
		}
	}
}

func (e *Engine) loopSpecial() {
	// 초기 상태 표시
	targetStr := "보관"
	if e.targetLevel > 0 {
		targetStr = fmt.Sprintf("+%d까지 강화", e.targetLevel)
	}
	overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n목표: %s\n\n📋 프로필 확인 중...", targetStr)

	// 시작 시 프로필 정보 표시 (Run()에서 이미 조회한 sessionProfile 사용)
	// 중복 /프로필 전송 방지
	if e.sessionProfile != nil && e.sessionProfile.SwordName != "" {
		fmt.Printf("📋 현재 보유 검: [+%d] %s\n", e.sessionProfile.Level, e.sessionProfile.SwordName)
	}

	overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n목표: %s\n쓰레기: 0회", targetStr)
	fmt.Println()

	retryCount := 0
	const maxRetries = 3

	for e.running {
		if e.checkStop() {
			return
		}

		// v3 흐름: /강화 먼저 → 아이템 이름 확인 → 특수면 계속, 아니면 /판매
		// 1. /강화 시도 (현재 검 강화하면서 아이템 이름 확인)
		overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n쓰레기: %d회\n📤 /강화 전송...", e.sessionStats.trashCount)
		e.sendCommand("/강화")

		// 응답 대기
		overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n쓰레기: %d회\n⏳ 응답 대기...", e.sessionStats.trashCount)

		// 결과 확인 (응답 변경 감지 + 재시도 로직)
		var text string
		var state *GameState
		readSuccess := false

		for retry := 0; retry < maxRetries && !readSuccess; retry++ {
			if retry > 0 {
				fmt.Printf("  🔄 재시도 %d/%d...\n", retry+1, maxRetries)
				overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n🔄 재시도 %d/%d", retry+1, maxRetries)
			}

			overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n쓰레기: %d회\n🔍 채팅창 분석...", e.sessionStats.trashCount)
			// 응답이 변경될 때까지 대기 (최대 5초)
			text = e.readChatTextWaitForChange(5 * time.Second)

			// 텍스트가 비어있으면 재시도
			if text == "" {
				continue
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
			overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n⚠️ 읽기 실패 %d회\n채팅창 확인!", retryCount)

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

		// 아이템 이름으로 타입 재판별 (ParseOCRText에서 실패한 경우 보완)
		// state.ItemType이 "none"이면 아직 판별 안됨 → itemName으로 다시 시도
		if state.ItemType == "none" && itemName != "" {
			state.ItemType = DetermineItemType(itemName)
		}

		// 디버그: 아이템 타입 출력
		fmt.Printf("  📋 감지: [%s] %s\n", state.ItemType, itemName)

		// 2. 강화 결과 확인 - 파괴되었으면 새 아이템 받음, 다음 루프
		if state.LastResult == "destroy" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			fmt.Printf("  💥 파괴됨 [%s] → 새 아이템 대기\n", itemName)
			overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n쓰레기: %d회\n💥 파괴 → 새 아이템", e.sessionStats.trashCount)
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// 3. 특수면 성공!
		if state.ItemType == "special" {
			overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n🎉 특수 발견!\n[%s]\n\n📋 판단: 특수 → 보관/강화", itemName)
			fmt.Printf("\n🎉 특수 아이템 발견! [%s]\n", itemName)
			logger.Info("특수 아이템 발견: %s", itemName)

			// 텔레메트리: 특수 아이템 발견 즉시 전송
			e.telem.RecordFarmingWithItem(itemName, "special")
			e.telem.RecordSword()
			e.telem.TrySend()
			e.sessionStats.specialCount++

			// 강화 목표가 있으면 강화 진행
			if e.targetLevel > 0 {
				// 현재 레벨 확인 (공통 헬퍼 사용)
				currentLevel := e.ExtractCurrentLevel(state)

				// 이미 목표 달성했으면 완료 (공통 헬퍼 사용)
				if e.IsTargetReached(currentLevel) {
					fmt.Printf("✅ 이미 목표 달성! [%s] +%d\n", itemName, currentLevel)
					overlay.UpdateStatus("⭐ 특수 강화 완료!\n[%s] +%d", itemName, currentLevel)
					e.telem.TrySend()
					return
				}

				fmt.Printf("📈 목표 +%d까지 강화를 시작합니다... (현재 +%d)\n", e.targetLevel, currentLevel)
				overlay.UpdateStatus("⭐ 특수 강화 중\n[%s] +%d\n목표: +%d", itemName, currentLevel, e.targetLevel)

				// 골드 체크
				if e.sessionProfile != nil && e.sessionProfile.Gold < 1000 {
					fmt.Println("⚠️ 골드가 부족하여 강화를 진행할 수 없습니다.")
					e.telem.TrySend()
					return
				}

				// 강화 진행 (공통 헬퍼 사용)
				result := e.EnhanceToTarget(itemName, currentLevel)
				if result.Success {
					fmt.Printf("✅ 강화 완료! [%s] +%d\n", itemName, result.FinalLevel)
					overlay.UpdateStatus("⭐ 특수 강화 완료!\n[%s] +%d", itemName, result.FinalLevel)
					e.telem.TrySend()
					return // 목표 달성 → 종료
				} else {
					// 파괴됨 → 다시 특수 아이템 찾기
					fmt.Printf("💥 강화 중 파괴됨 (최종 레벨: +%d) → 다시 특수 아이템 찾기\n", result.FinalLevel)
					overlay.UpdateStatus("💥 특수 파괴됨\n다시 특수 찾는 중...")
					e.telem.TrySend()
					time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
					continue // 루프 계속 → 특수 아이템 다시 찾기
				}
			} else {
				// 강화 목표 없으면 (보관만) 바로 종료
				fmt.Printf("✅ 특수 아이템 보관 완료! [%s]\n", itemName)
				overlay.UpdateStatus("⭐ 특수 보관 완료!\n[%s]", itemName)
				e.telem.TrySend()
				return
			}
		}

		// 4. 쓰레기/일반/미판별이면 /판매로 새 아이템 받기 (v3 변경점)
		// "none"도 포함: 타입 판별 실패 시 계속 강화하면 안되므로 판매 처리
		if state.ItemType == "trash" || state.ItemType == "normal" || state.ItemType == "unknown" || state.ItemType == "none" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			displayName := itemName
			if displayName == "" {
				displayName = GetItemTypeLabel(state.ItemType)
			}
			overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n쓰레기: %d회\n🗑️ %s\n\n📋 판단: %s → 판매", e.sessionStats.trashCount, displayName, GetItemTypeLabel(state.ItemType))
			fmt.Printf("  🗑️ [%s] → /판매\n", displayName)

			// /판매로 새 아이템 받기
			e.sendCommand("/판매")
			// 판매 응답 대기 (응답 없이 다음 /강화 보내면 꼬임)
			e.readChatTextWaitForChange(5 * time.Second)
			time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
			continue
		}

		// 5. 예상치 못한 타입 - 안전하게 판매 처리 (무한 강화 방지)
		fmt.Printf("  ❓ 예상치 못한 아이템 타입: [%s] - 판매 처리\n", state.ItemType)
		overlay.UpdateStatus("⭐ 특수 아이템 뽑기\n❓ 타입 불명 → 판매")
		e.sendCommand("/판매")
		// 판매 응답 대기
		e.readChatTextWaitForChange(5 * time.Second)
		time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))
	}
}

func (e *Engine) loopGoldMine() {
	// v3: 세션 초기화
	startGold := e.readCurrentGold()
	e.telem.InitSession(startGold)
	overlay.UpdateStatus("💰 골드 채굴 모드\n목표: +%d\n사이클: 0\n수익: 0G", e.targetLevel)

	// 시작 시 프로필 정보 표시 (Run()에서 이미 조회한 sessionProfile 사용)
	// 중복 /프로필 전송 방지
	if e.sessionProfile != nil && e.sessionProfile.SwordName != "" {
		fmt.Printf("📋 현재 보유 검: [+%d] %s\n", e.sessionProfile.Level, e.sessionProfile.SwordName)

		// 아이템 타입 확인
		itemType := DetermineItemType(e.sessionProfile.SwordName)
		fmt.Printf("   아이템 타입: %s\n", GetItemTypeLabel(itemType))

		// 이미 목표 달성한 경우 바로 판매 (일반 아이템만)
		if e.IsTargetReached(e.sessionProfile.Level) {
			if itemType == "special" {
				fmt.Printf("✅ 목표 달성! 특수 아이템 [%s] +%d → 보관\n", e.sessionProfile.SwordName, e.sessionProfile.Level)
				overlay.UpdateStatus("💰 골드 채굴\n✅ 특수 +%d 보관!", e.sessionProfile.Level)
				e.telem.TrySend()
				return // 특수 아이템은 판매하지 않음
			}

			fmt.Printf("✅ 이미 목표 달성! 현재 +%d → 바로 판매\n", e.sessionProfile.Level)
			overlay.UpdateStatus("💰 골드 채굴\n✅ 이미 +%d 보유!\n💵 판매 진행", e.sessionProfile.Level)
			e.sendCommand("/판매")
			saleText := e.readChatTextWaitForChange(5 * time.Second)
			saleResult := ExtractSaleResult(saleText)
			if saleResult != nil && saleResult.SaleGold > 0 {
				e.totalGold += saleResult.SaleGold
				fmt.Printf("💰 판매 완료: +%sG\n", FormatGold(saleResult.SaleGold))
			}
		}
	}
	fmt.Println()

	// 판매 후 +0 검 정보 추적 (다음 사이클에서 farmForGoldMine 스킵용)
	var pendingZeroSword struct {
		name     string
		itemType string
		valid    bool
	}

	// 세션 시작 시 기존 보유 검 정보 (목표 미달이지만 0강 이상인 경우)
	var pendingExistingSword struct {
		name     string
		itemType string
		level    int
		valid    bool
	}

	// 세션 시작 시 이미 보유한 검이 있고, 목표 미달이면 바로 강화 이어가기
	if e.sessionProfile != nil && e.sessionProfile.Level > 0 && !e.IsTargetReached(e.sessionProfile.Level) {
		pendingExistingSword.name = e.sessionProfile.SwordName
		pendingExistingSword.itemType = DetermineItemType(e.sessionProfile.SwordName)
		pendingExistingSword.level = e.sessionProfile.Level
		pendingExistingSword.valid = true
		fmt.Printf("📋 기존 검 +%d 보유 중 → 목표 +%d까지 강화 이어가기\n", e.sessionProfile.Level, e.targetLevel)
	}

	for e.running {
		if e.checkStop() {
			return
		}

		e.cycleStartTime = time.Now()
		e.cycleCount++

		var itemName, itemType string
		var itemLevel int
		var found bool

		// 우선순위 1: 세션 시작 시 기존 보유 검 (목표 미달이지만 0강 이상)
		if pendingExistingSword.valid {
			itemName = pendingExistingSword.name
			itemType = pendingExistingSword.itemType
			itemLevel = pendingExistingSword.level
			found = true
			pendingExistingSword.valid = false // 사용 후 초기화
			fmt.Printf("  📦 기존 보유 검 사용: %s +%d → 강화 이어가기\n", itemName, itemLevel)
		} else if pendingZeroSword.valid {
			// 우선순위 2: 이전 판매로 받은 +0 검
			itemName = pendingZeroSword.name
			itemType = pendingZeroSword.itemType
			itemLevel = 0
			found = true
			pendingZeroSword.valid = false // 사용 후 초기화
			fmt.Printf("  📦 이전 판매로 받은 +0 검: %s → 바로 강화 시작\n", itemName)
		} else {
			// 1. 파밍 (아이템 이름, 타입, 레벨 반환 - 레벨 정보 추가됨)
			overlay.UpdateStatus("💰 골드 채굴 #%d\n🔍 파밍 중...\n누적: %sG", e.cycleCount, FormatGold(e.totalGold))
			itemName, itemType, itemLevel, found = e.farmForGoldMine()
		}

		if !found {
			e.ReportCycleFailed()
			overlay.UpdateStatus("💰 골드 채굴 #%d\n❌ 파밍 실패\n누적: %sG", e.cycleCount, FormatGold(e.totalGold))
			continue
		}

		// 아이템 타입 표시
		typeLabel := GetItemTypeLabel(itemType)
		if itemType == "special" {
			fmt.Printf("🎉 특수 아이템 발견: %s +%d\n", itemName, itemLevel)
		}

		// 2. 목표 도달 확인 (공통 헬퍼 사용)
		// 이미 목표 달성이면 강화 스킵하고 바로 판매
		var finalLevel int
		var enhanceCost int

		// 강화 시작 전 골드 측정 (순수익 계산용)
		goldBeforeEnhance := e.readCurrentGold()

		if e.IsTargetReached(itemLevel) {
			fmt.Printf("✅ 파밍에서 이미 목표 도달: %s +%d\n", itemName, itemLevel)
			finalLevel = itemLevel
			enhanceCost = 0
		} else {
			// 3. 강화 (공통 헬퍼 사용 - 시작 레벨 전달)
			overlay.UpdateStatus("💰 골드 채굴 #%d\n⚔️ 강화 중: %s +%d (%s)\n목표: +%d\n누적: %sG",
				e.cycleCount, itemName, itemLevel, typeLabel, e.targetLevel, FormatGold(e.totalGold))

			result := e.EnhanceToTarget(itemName, itemLevel)
			if !result.Success {
				if result.Destroyed {
					fmt.Printf("💥 강화 중 파괴: %s (최종 +%d)\n", itemName, result.FinalLevel)

					// 파괴 시 새 검 정보가 있으면 다음 사이클용으로 저장
					if result.NewSwordName != "" {
						pendingZeroSword.name = result.NewSwordName
						pendingZeroSword.itemType = result.NewSwordType
						pendingZeroSword.valid = true
						fmt.Printf("  📦 새 검 획득: [+0] %s\n", result.NewSwordName)
					}
				}
				e.ReportCycleFailed()
				continue
			}
			finalLevel = result.FinalLevel

			// 강화 비용 계산 (음수 방지)
			goldAfterEnhance := e.readCurrentGold()
			if goldBeforeEnhance > 0 && goldAfterEnhance > 0 {
				calculatedCost := goldBeforeEnhance - goldAfterEnhance
				if calculatedCost >= 0 {
					enhanceCost = calculatedCost
				}
			}
		}

		// 4. 판매 (목표 레벨 도달 시)
		goldBeforeSale := e.readCurrentGold()

		overlay.UpdateStatus("💰 골드 채굴 #%d\n💵 판매 중: %s +%d\n누적: %sG\n\n📋 판단: +%d 달성 → 판매",
			e.cycleCount, itemName, finalLevel, FormatGold(e.totalGold), e.targetLevel)
		e.sendCommand("/판매")

		// 판매 응답 대기 및 골드 추출
		saleText := e.readChatTextWaitForChange(5 * time.Second)
		saleResult := ExtractSaleResult(saleText)

		var saleGold, currentGold int
		if saleResult != nil {
			// SaleGold가 -1이면 파싱 실패 → 0으로 처리
			if saleResult.SaleGold > 0 {
				saleGold = saleResult.SaleGold
			}
			if saleResult.CurrentGold > 0 {
				currentGold = saleResult.CurrentGold
			}

			// 새 검이 +0이면 다음 사이클에서 farmForGoldMine 스킵
			// NewSwordLvl이 0 또는 -1(파싱실패)이고 이름이 있으면 → +0 검으로 처리
			// (게임에서 판매 후 새 검은 항상 +0)
			if saleResult.NewSwordName != "" {
				pendingZeroSword.name = saleResult.NewSwordName
				pendingZeroSword.itemType = DetermineItemType(saleResult.NewSwordName)
				pendingZeroSword.valid = true
			}
		}

		// 폴백: 직접 추출 실패 시 기존 방식 사용
		// saleGold가 0 이하면 폴백 시도 (파싱 실패 -1 포함)
		if saleGold <= 0 {
			endGold := e.readCurrentGold()
			if endGold > 0 && goldBeforeSale > 0 {
				// 정상적인 경우만 계산 (음수 방지)
				calculatedSale := endGold - goldBeforeSale
				if calculatedSale >= 0 {
					saleGold = calculatedSale
					currentGold = endGold
				}
			}
		}

		// 5. 순수익 계산 (판매 수익 - 강화 비용)
		netProfit := saleGold - enhanceCost

		// 6. 사이클 통계
		cycleTime := time.Since(e.cycleStartTime)
		e.totalGold += netProfit // 순수익으로 누적

		// v3 텔레메트리 기록 (공통 헬퍼 사용) - 서버에는 판매 수익 보고
		e.ReportGoldMineCycle(itemName, finalLevel, saleGold, currentGold, enhanceCost, cycleTime.Seconds())

		// 세션 통계 업데이트 - 순수익 기준
		e.sessionStats.cycleTimeSum += cycleTime.Seconds()
		e.sessionStats.cycleGoldSum += netProfit

		// 사이클 완료 상태 업데이트 - 순수익 상세 표시
		overlay.UpdateStatus("💰 골드 채굴 #%d ✅\n%s +%d\n💵 판매: +%sG\n⚔️ 강화비: -%sG\n📊 순수익: %+sG\n\n누적: %sG",
			e.cycleCount, itemName, finalLevel,
			FormatGold(saleGold), FormatGold(enhanceCost), FormatGold(netProfit), FormatGold(e.totalGold))

		fmt.Printf("📦 사이클 #%d: %.1f초 | 판매 +%sG - 강화 %sG = 순수익 %sG | 누적: %sG [%s +%d %s]\n",
			e.cycleCount, cycleTime.Seconds(), FormatGold(saleGold), FormatGold(enhanceCost), FormatGold(netProfit), FormatGold(e.totalGold), itemName, finalLevel, typeLabel)
	}
}

func (e *Engine) loopBattle() {
	fmt.Println()

	// 시작 시 프로필 정보 표시 (Run()에서 이미 조회한 sessionProfile 사용)
	// 중복 /프로필 전송 방지
	if e.sessionProfile == nil || e.sessionProfile.Level < 0 {
		fmt.Println("❌ 프로필을 읽을 수 없습니다. 다시 시도하세요.")
		return
	}

	// 배틀 모드에서 사용할 myProfile에 sessionProfile 복사
	e.myProfile = e.sessionProfile

	fmt.Printf("📋 내 프로필: +%d %s (%d승 %d패)\n",
		e.myProfile.Level, e.myProfile.SwordName, e.myProfile.Wins, e.myProfile.Losses)
	fmt.Printf("🎯 타겟 범위: +%d ~ +%d\n",
		e.myProfile.Level+1, e.myProfile.Level+e.cfg.BattleLevelDiff)
	fmt.Println()

	// v2: 세션 초기화
	startGold := e.readCurrentGold()
	e.telem.InitSession(startGold)

	// 적합한 타겟 목록 (배틀 루프 밖에서 유지, 소진되면 다시 조회)
	var candidates []*RankingEntry

	// 배틀 루프
	for e.running {
		if e.checkStop() {
			return
		}

		e.cycleCount++

		// 타겟 목록이 비었으면 새로 조회
		if len(candidates) == 0 {
			fmt.Println("🔄 타겟 목록 갱신 중...")

			// 2. 랭킹에서 유저 목록 가져오기
			e.SaveLastChatText()
			e.sendCommand("/랭킹")
			// 랭킹은 다른 유저 이름이 포함되므로 Raw 사용
			rankingText := e.waitForResponseRaw(5 * time.Second)
			entries := ParseRanking(rankingText)
			usernames := ExtractUsernamesFromRanking(entries)

			if len(usernames) == 0 {
				fmt.Println("⏳ 랭킹에서 유저를 찾을 수 없음, 30초 후 재시도...")
				if e.sleepWithHotkeyCheck(30 * time.Second) {
					return
				}
				continue
			}

			// 3. 모든 유저의 프로필 확인하여 적합한 타겟 목록 수집
			minTarget := e.myProfile.Level + 1
			maxTarget := e.myProfile.Level + e.cfg.BattleLevelDiff

			fmt.Printf("🔍 %d명의 유저 프로필 확인 중... (타겟: +%d ~ +%d)\n", len(usernames), minTarget, maxTarget)

			for _, username := range usernames {
				if e.checkStop() {
					return
				}

				// 자기 자신은 스킵
				if username == e.myProfile.Name {
					continue
				}

				profile := e.CheckOtherProfile(username)
				if profile == nil || profile.Level <= 0 {
					fmt.Printf("   ⚠️ %s: 프로필 조회 실패 또는 0레벨\n", username)
					time.Sleep(1 * time.Second)
					continue
				}

				if profile.Level >= minTarget && profile.Level <= maxTarget {
					candidates = append(candidates, &RankingEntry{
						Username: username,
						Level:    profile.Level,
					})
					fmt.Printf("   ✅ %s: +%d (적합!)\n", username, profile.Level)
				} else {
					fmt.Printf("   ❌ %s: +%d (범위 외)\n", username, profile.Level)
				}

				time.Sleep(1 * time.Second) // 프로필 조회 간격
			}

			if len(candidates) == 0 {
				fmt.Println("⏳ 적합한 타겟 없음, 30초 후 재시도...")
				if e.sleepWithHotkeyCheck(30 * time.Second) {
					return
				}
				continue
			}

			fmt.Printf("📋 적합한 타겟 %d명 발견\n", len(candidates))
		}

		// 적합한 타겟 중 가장 레벨이 낮은 타겟 선택 (역배 확률 최대화)
		// 같은 타겟을 계속 사용 (제거하지 않음)
		var target *RankingEntry
		target = candidates[0]
		for _, c := range candidates[1:] {
			if c.Level < target.Level {
				target = c
			}
		}

		// 4. 타겟과 배틀
		// 승률 계산
		winRate := 0.0
		if e.battleWins+e.battleLosses > 0 {
			winRate = float64(e.battleWins) / float64(e.battleWins+e.battleLosses) * 100
		}

		fmt.Printf("⚔️ #%d: %s (+%d) vs 나 (+%d) [%s]\n",
			e.cycleCount, target.Username, target.Level, e.myProfile.Level, e.myProfile.SwordName)
		overlay.UpdateStatus("⚔️ 자동 배틀 #%d\n타겟: %s +%d\n내 레벨: +%d\n\n💰 수익: %sG\n📊 승률: %.1f%% (%d승 %d패)",
			e.cycleCount, target.Username, target.Level, e.myProfile.Level,
			FormatGold(e.totalGold), winRate, e.battleWins, e.battleLosses)

		e.SaveLastChatText()
		// 배틀 명령어는 다단계로 전송 (카카오톡 인식 안정성)
		// /배틀 → 0.3초 → 엔터(줄바꿈) → 0.3초 → @이름 → 엔터,엔터(전송)
		e.sendCommandOnce("/배틀")
		time.Sleep(300 * time.Millisecond)
		e.appendAndSend(target.Username)
		// 배틀 결과는 상대 이름 포함 → filterMyMessages가 패배 결과를 제거할 수 있으므로 Raw 사용
		resultText := e.waitForResponseRaw(5 * time.Second)

		// 응답이 없으면 재시도
		if resultText == "" {
			for retry := 0; retry < 3 && e.running; retry++ {
				time.Sleep(1 * time.Second)
				resultText = e.waitForResponseRaw(3 * time.Second)
				if resultText != "" {
					break
				}
			}
		}

		// 빈 결과 스킵 (가짜 패배 방지)
		if resultText == "" {
			fmt.Println("   ⚠️ 배틀 결과를 읽을 수 없음, 스킵")
			time.Sleep(2 * time.Second)
			continue
		}

		// 상대방 0강 감지 → 해당 타겟 제거 후 다음 타겟으로
		if DetectBattleZeroLevel(resultText) {
			fmt.Printf("   ⚠️ %s: 상대 검이 0강 → 타겟에서 제거\n", target.Username)
			// candidates에서 해당 타겟 제거
			for i, c := range candidates {
				if c.Username == target.Username {
					candidates = append(candidates[:i], candidates[i+1:]...)
					break
				}
			}
			time.Sleep(1 * time.Second)
			continue
		}

		// 배틀 횟수 제한 확인 (하루 10회 제한)
		if DetectBattleLimit(resultText) {
			// 최종 승률 계산
			finalWinRate := 0.0
			if e.battleWins+e.battleLosses > 0 {
				finalWinRate = float64(e.battleWins) / float64(e.battleWins+e.battleLosses) * 100
			}

			fmt.Println()
			fmt.Println("════════════════════════════════════════")
			fmt.Println("⏰ 오늘 배틀 횟수를 모두 사용했습니다 (10회/일)")
			fmt.Println("════════════════════════════════════════")
			fmt.Printf("📊 최종 전적: %d승 %d패 (승률 %.1f%%)\n", e.battleWins, e.battleLosses, finalWinRate)
			fmt.Printf("💰 총 수익: %sG\n", FormatGold(e.totalGold))
			fmt.Println("════════════════════════════════════════")
			fmt.Println()
			fmt.Println("엔터를 누르면 메뉴로 돌아갑니다...")

			overlay.UpdateStatus("⚔️ 자동 배틀 완료\n⏰ 일일 배틀 제한 도달\n\n📊 전적: %d승 %d패\n📈 승률: %.1f%%\n💰 총 수익: %sG",
				e.battleWins, e.battleLosses, finalWinRate, FormatGold(e.totalGold))

			// 사용자 입력 대기 후 메뉴 복귀
			fmt.Scanln()
			return
		}

		result := ParseBattleResult(resultText, e.myProfile.Name)

		goldChange := 0
		if result.Won {
			e.battleWins++
			goldChange = result.GoldEarned
			e.totalGold += goldChange

			// 승률 업데이트
			winRate = float64(e.battleWins) / float64(e.battleWins+e.battleLosses) * 100

			fmt.Printf("   → 🏆 승리! +%sG (역배 성공!)\n", FormatGold(goldChange))
			overlay.UpdateStatus("⚔️ 자동 배틀\n🏆 승리! +%sG\n\n💰 수익: %sG\n📊 승률: %.1f%% (%d승 %d패)",
				FormatGold(goldChange), FormatGold(e.totalGold), winRate, e.battleWins, e.battleLosses)
		} else {
			e.battleLosses++

			// 패배 시 골드 손실: 배틀 결과에 표시된 골드(승자 획득량)를 손실로 간주
			if result.GoldEarned > 0 {
				goldChange = -result.GoldEarned
				e.totalGold -= result.GoldEarned
			}

			// 승률 업데이트
			winRate = float64(e.battleWins) / float64(e.battleWins+e.battleLosses) * 100

			if result.GoldEarned > 0 {
				fmt.Printf("   → 💔 패배... -%sG\n", FormatGold(result.GoldEarned))
			} else {
				fmt.Println("   → 💔 패배...")
			}
			overlay.UpdateStatus("⚔️ 자동 배틀\n💔 패배...\n\n💰 수익: %sG\n📊 승률: %.1f%% (%d승 %d패)",
				FormatGold(e.totalGold), winRate, e.battleWins, e.battleLosses)
		}

		// 5. v3 텔레메트리 기록 (공통 헬퍼 사용) - goldChange는 승리 시 양수, 패배 시 음수
		currentGold := e.readCurrentGold()
		e.ReportBattleCycle(e.myProfile.SwordName, e.myProfile.Level, target.Level, result.Won, goldChange, currentGold)

		// 6. 현재 통계 출력 (공통 헬퍼 사용)
		PrintBattleStats(e.battleWins, e.battleLosses, e.totalGold)

		// 7. 프로필 갱신은 생략 (같은 타겟 계속 사용하므로 불필요)

		// 8. 쿨다운
		time.Sleep(time.Duration(e.cfg.BattleCooldown * float64(time.Second)))
	}
}

// ResetLastChatText 마지막 채팅 텍스트 초기화 (새 응답 감지를 위해)
// 중요한 명령어 전송 전에 호출하여 응답 대기가 제대로 작동하도록 함
func (e *Engine) ResetLastChatText() {
	e.lastRawChatText = ""
}

// SaveLastChatText 현재 채팅 텍스트를 저장 (새 응답만 감지하기 위해)
// 다른 유저 프로필 조회 등에서 명령어 전송 전에 호출
// ResetLastChatText와 달리 현재 채팅을 저장하여 새 응답만 추출 가능
func (e *Engine) SaveLastChatText() {
	e.lastRawChatText = e.readChatTextRaw()
}

// readChatText 화면에서 텍스트 읽기 (클립보드 방식)
// 내 메시지만 필터링하여 반환 (다른 사람 메시지 무시)
func (e *Engine) readChatText() string {
	text := e.readChatClipboard()
	// 내 메시지만 필터링 (프로필이 있는 경우)
	return e.filterMyMessages(text)
}

// readChatTextRaw 화면에서 텍스트 읽기 (필터 없음)
// 랭킹, 다른 유저 프로필 등 다른 사람 정보가 필요할 때 사용
func (e *Engine) readChatTextRaw() string {
	return e.readChatClipboard()
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

	logger.ChatText(text) // 새로운 채팅만 로깅
	return text
}


// readChatTextWaitForChange 응답이 올 때까지 대기하며 텍스트 읽기
// RAW 텍스트로 변경 감지 + 필터된 텍스트도 변경 확인 (이중 체크)
// 다른 유저 메시지로만 변경된 경우 계속 대기 (내 응답이 올 때까지)
func (e *Engine) readChatTextWaitForChange(maxWait time.Duration) string {
	startTime := time.Now()
	pollInterval := 500 * time.Millisecond
	initialWait := 1 * time.Second // 봇 응답 대기 (명령어가 채팅에 반영된 후 봇이 응답할 시간 확보)
	lastFiltered := e.filterMyMessages(e.lastRawChatText)

	// 초기 대기: sendCommand 직후 즉시 폴링하면 사용자 명령어만 감지되어
	// 봇 응답 없이 반환될 수 있음 (stale data 문제)
	// 대기 중에도 이벤트 펌핑
	for elapsed := time.Duration(0); elapsed < initialWait; elapsed += 100 * time.Millisecond {
		overlay.PumpEvents()
		time.Sleep(100 * time.Millisecond)
	}

	for time.Since(startTime) < maxWait {
		// 대기 중에도 오버레이 이벤트 처리
		overlay.PumpEvents()

		rawText := e.readChatClipboard()
		if rawText == "" {
			time.Sleep(pollInterval)
			continue
		}

		if rawText != e.lastRawChatText {
			e.lastRawChatText = rawText
			filtered := e.filterMyMessages(rawText)
			// 내 메시지가 실제로 변경된 경우에만 반환
			if filtered != lastFiltered {
				return filtered
			}
			// 다른 유저 메시지로 인한 변경 → 계속 대기
		}

		time.Sleep(pollInterval)
	}

	return ""
}

// waitForResponse 플레이봇 응답 대기 (최대 maxWait 동안)
// 명령어 전송 후 응답이 올 때까지 대기
// 새로운 부분만 반환 (내 메시지 필터링됨)
func (e *Engine) waitForResponse(maxWait time.Duration) string {
	return e.waitForResponseInternal(maxWait, false)
}

// waitForResponseRaw 플레이봇 응답 대기 (필터 없음)
// 랭킹, 다른 유저 프로필 등 다른 사람 정보가 필요할 때 사용
func (e *Engine) waitForResponseRaw(maxWait time.Duration) string {
	return e.waitForResponseInternal(maxWait, true)
}

// waitForResponseInternal 응답 대기 내부 구현
// RAW 텍스트로 변경 감지 + 필터된 텍스트도 변경 확인
// raw=true면 RAW 변경 즉시 반환, false면 필터 텍스트 변경 시 반환
func (e *Engine) waitForResponseInternal(maxWait time.Duration, raw bool) string {
	startTime := time.Now()
	pollInterval := 500 * time.Millisecond
	initialWait := 1 * time.Second
	lastFiltered := e.filterMyMessages(e.lastRawChatText)

	// 최소 대기 (명령어 처리 시간) - 대기 중에도 이벤트 펌핑
	for elapsed := time.Duration(0); elapsed < initialWait; elapsed += 100 * time.Millisecond {
		overlay.PumpEvents()
		time.Sleep(100 * time.Millisecond)
	}

	for time.Since(startTime) < maxWait {
		// 대기 중에도 오버레이 이벤트 처리 (버튼 클릭 감지)
		overlay.PumpEvents()

		rawText := e.readChatClipboard()
		if rawText == "" {
			time.Sleep(pollInterval)
			continue
		}

		if rawText != e.lastRawChatText {
			e.lastRawChatText = rawText
			if raw {
				return rawText
			}
			filtered := e.filterMyMessages(rawText)
			if filtered != lastFiltered {
				return filtered
			}
			// 다른 유저 메시지로 인한 변경 → 계속 대기
		}

		time.Sleep(pollInterval)
	}

	return ""
}

// filterMyMessages 내 메시지만 필터링 (다른 유저 영역 제거 방식)
// 기존 "마지막 섹션 선택" 방식의 문제:
//   같은 채팅창에 성공(+9→+10)과 유지(+10)가 동시에 잡힐 때
//   마지막 @myName(유지)만 반환 → 성공 결과 유실 → 목표 도달 감지 실패
// 개선: 다른 유저의 영역만 제거하고, 내 메시지는 모두 보존
func (e *Engine) filterMyMessages(text string) string {
	if e.sessionProfile == nil || e.sessionProfile.Name == "" {
		return text // 프로필 없으면 전체 반환
	}

	myName := e.sessionProfile.Name // "@행복사랑평화" 형식
	lines := strings.Split(text, "\n")

	// 다른 유저 영역 제거, 내 영역은 모두 보존
	// 상태 머신: @가 포함된 줄에서 유저 전환 감지
	// - @myName 포함 → 내 영역 (포함)
	// - @있지만 myName 없음 → 다른 유저 영역 (제거)
	// - @없음 → 현재 상태 유지 (이전 영역에 속하는 상세 메시지)
	var result []string
	inOtherSection := false

	for _, line := range lines {
		hasAt := strings.Contains(line, "@")
		hasMy := strings.Contains(line, myName)

		if hasAt {
			if hasMy {
				// 내 영역으로 전환 (결과, 속보 등)
				inOtherSection = false
			} else {
				// 다른 유저 영역으로 전환
				// 예: "플레이봇 @권혁진 〖결과〗", "한지원 @플레이봇 강화"
				inOtherSection = true
				continue // 이 줄도 제거
			}
		}

		if !inOtherSection {
			result = append(result, line)
		}
	}

	if len(result) == 0 {
		return text
	}

	return strings.Join(result, "\n")
}

func (e *Engine) farmUntilSpecial() bool {
	_, found := e.farmUntilSpecialWithName()
	return found
}

// farmForGoldMine 골드 채굴 모드용 파밍 - 모든 아이템 타입 반환 (파괴하지 않음)
// 로직: /판매 시도 → 판매 불가면 현재 아이템 유지 → 아이템 정보 반환
// 반환값: (itemName, itemType, itemLevel, found)
func (e *Engine) farmForGoldMine() (string, string, int, bool) {
	retryCount := 0
	const maxRetries = 3

	for e.running {
		if e.checkStop() {
			return "", "", 0, false
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

			// 2. 판매 불가 체크 (0강 아이템은 판매 불가) - 현재 아이템 유지하고 강화 진행
			if CannotSell(text) {
				// 0강 아이템 - 파괴하지 않고 /강화로 진행
				// 먼저 /강화를 보내서 아이템 정보 확인
				e.sendCommand("/강화")
				time.Sleep(time.Duration(e.cfg.TrashDelay * float64(time.Second)))

				// 강화 결과 읽기 (응답 대기)
				enhanceText := e.readChatTextWaitForChange(5 * time.Second)
				enhanceState := ParseOCRText(enhanceText)

				if enhanceState != nil {
					itemName := enhanceState.ItemName
					if itemName == "" {
						itemName = ExtractItemName(enhanceText)
					}
					itemType := enhanceState.ItemType
					e.telem.RecordFarmingWithItem(itemName, itemType)

					// 파괴되었으면 다시 파밍
					if enhanceState.LastResult == "destroy" {
						fmt.Printf("  💥 0강 아이템 파괴 - 다음 아이템\n")
						continue
					}

					// 성공 또는 유지 시 현재 레벨 반환 (공통 헬퍼 사용)
					currentLevel := e.ExtractCurrentLevel(enhanceState)
					if currentLevel == 0 && enhanceState.LastResult != "hold" {
						currentLevel = 1 // 0강에서 강화 성공하면 최소 1강
					}

					fmt.Printf("  📦 0강 아이템 강화: %s → +%d\n", itemName, currentLevel)
					return itemName, itemType, currentLevel, true
				}
				continue
			}

			// 3. 새 검 획득 체크
			state = ParseOCRText(text)
			if state != nil {
				readSuccess = true
				retryCount = 0
			}
		}

		// 계속 실패하면 경고
		if !readSuccess {
			retryCount++
			fmt.Printf("  ⚠️ 채팅창 읽기 %d회 연속 실패 - 재시도\n", retryCount)

			if retryCount >= 5 {
				fmt.Println("\n❌ 채팅창 읽기가 계속 실패합니다! 카카오톡 창 상태를 확인하세요.")
				time.Sleep(3 * time.Second)
				retryCount = 0
			} else {
				time.Sleep(1 * time.Second)
			}
			continue
		}

		// 아이템 이름 추출
		itemName := state.ItemName
		if itemName == "" {
			itemName = ExtractItemName(text)
		}
		itemType := state.ItemType

		// 현재 레벨 추출 (공통 헬퍼 사용) - 새 검은 0강
		currentLevel := e.ExtractCurrentLevel(state)

		// 텔레메트리 기록
		e.telem.RecordFarmingWithItem(itemName, itemType)

		// 아이템 타입별 통계 기록
		if itemType == "special" {
			e.sessionStats.specialCount++
			fmt.Printf("🎉 특수 아이템! [%s] +%d\n", itemName, currentLevel)
		} else {
			e.sessionStats.trashCount++
		}

		// 모든 아이템 타입 반환 (골드 채굴은 아이템 가리지 않음) + 레벨 포함
		return itemName, itemType, currentLevel, true
	}
	return "", "", 0, false
}

// farmUntilSpecialWithName 특수 아이템을 찾을 때까지 파밍하고 아이템 이름 반환
// 로직: /판매 → 채팅창 읽기 → 쓰레기면 /강화(파괴) → 반복, 특수면 반환
func (e *Engine) farmUntilSpecialWithName() (string, bool) {
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

		// 4. 특수 아이템이면 반환 (강화 모드로 전환)
		if state.ItemType == "special" {
			e.telem.RecordFarmingWithItem(itemName, "special")
			e.sessionStats.specialCount++
			fmt.Printf("🎉 특수 발견! [%s]\n", itemName)
			overlay.UpdateStatus("💰 골드 채굴 #%d\n🎉 특수 발견!\n[%s]\n\n📋 판단: 특수 → 강화", e.cycleCount, itemName)
			return itemName, true
		}

		// 5. 쓰레기/일반 아이템이면 /강화로 파괴하고 반복
		if state.ItemType == "trash" || state.ItemType == "normal" || state.ItemType == "unknown" {
			e.telem.RecordFarmingWithItem(itemName, state.ItemType)
			e.sessionStats.trashCount++
			displayName := itemName
			if displayName == "" {
				displayName = GetItemTypeLabel(state.ItemType)
			}
			overlay.UpdateStatus("💰 골드 채굴 #%d\n🗑️ %s\n\n📋 판단: %s → 파괴\n쓰레기: %d회", e.cycleCount, displayName, GetItemTypeLabel(state.ItemType), e.sessionStats.trashCount)
			// 쓰레기는 /강화로 파괴 (0강이므로 바로 파괴됨)
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
			// 실제 게임 상태에서 레벨 읽기 (ResultLevel이 있으면 사용, 없으면 수동 증가)
			if state.ResultLevel > 0 {
				currentLevel = state.ResultLevel
			} else {
				currentLevel++
			}
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
			// 유지 시에도 ResultLevel 확인 (현재 레벨 동기화)
			if state.ResultLevel > 0 && state.ResultLevel != currentLevel {
				currentLevel = state.ResultLevel
			}
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

// sendCommandOnce 엔터 1번만 누르는 명령어 전송
// 입력창 클리어 후 텍스트 입력, 엔터 1번 (줄바꿈만, 전송 안됨)
func (e *Engine) sendCommandOnce(cmd string) {
	input.SendCommandOnce(e.cfg.ClickX, e.cfg.ClickY, cmd)
}

// appendAndSend 기존 입력에 텍스트 추가 후 전송
// 입력창을 클리어하지 않고 텍스트를 추가한 뒤 전송 (엔터 2번)
func (e *Engine) appendAndSend(text string) {
	input.AppendAndSend(e.cfg.ClickX, e.cfg.ClickY, text)
}

func (e *Engine) checkStop() bool {
	// F9 핫키 체크
	if input.CheckF9Pressed() {
		fmt.Println("\n⏹️ F9 종료!")
		infoX := e.cfg.ClickX - 20
		infoY := e.cfg.ClickY - 20 + e.cfg.OverlayInputHeight + 5
		overlay.ShowInfoPanel(infoX, infoY, "⏹ 종료 중...")
		e.running = false
		return true
	}

	return !e.running
}

// sleepWithHotkeyCheck 대기 중에도 핫키 체크 (200ms 간격)
// 긴 Sleep 중에도 F9로 즉시 종료 가능
func (e *Engine) sleepWithHotkeyCheck(duration time.Duration) bool {
	const checkInterval = 200 * time.Millisecond
	elapsed := time.Duration(0)
	for elapsed < duration {
		if e.checkStop() {
			return true // 종료 요청됨
		}
		sleepTime := checkInterval
		if duration-elapsed < checkInterval {
			sleepTime = duration - elapsed
		}
		time.Sleep(sleepTime)
		elapsed += sleepTime
	}
	return false // 정상 완료
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

	// 채팅 상태 초기화 (로그에 전체 이력 방지)
	initialText := e.readChatClipboard()
	if initialText != "" {
		e.lastRawChatText = initialText
		logger.ChatText(e.filterMyMessages(initialText))
	}

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
	PrintEnhanceRateTable(profile.Level)

	// 4. 목표별 성공 확률
	PrintTargetSuccessChance(profile.Level)

	// 5. 역배 기대값
	PrintUpsetAnalysis(profile.Level, profile.Gold)

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
