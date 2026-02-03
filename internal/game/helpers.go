package game

import (
	"fmt"
	"time"
)

// =============================================================================
// 공통 헬퍼 함수들
// loopEnhance, loopSpecial, loopGoldMine에서 공유하는 로직
// =============================================================================

// ProfileCheckResult 프로필 확인 결과
type ProfileCheckResult struct {
	Level     int
	SwordName string
	Gold      int
	OK        bool
}

// CheckProfileLevel 프로필에서 현재 레벨과 검 이름 조회
// loopEnhance에서 추출한 공통 로직
func (e *Engine) CheckProfileLevel() ProfileCheckResult {
	// 이전 채팅 기록 초기화하여 새 응답 감지 보장
	e.ResetLastChatText()

	e.sendCommand("/프로필")
	profileText := e.waitForResponse(5 * time.Second)

	if profileText == "" {
		return ProfileCheckResult{OK: false}
	}

	profile := ParseProfile(profileText)
	if profile == nil || profile.Level < 0 {
		return ProfileCheckResult{OK: false}
	}

	return ProfileCheckResult{
		Level:     profile.Level,
		SwordName: profile.SwordName,
		Gold:      profile.Gold,
		OK:        true,
	}
}

// ExtractCurrentLevel GameState에서 현재 레벨 추출
// loopSpecial에서 추출한 공통 로직: ResultLevel > Level > 0 순서로 확인
func (e *Engine) ExtractCurrentLevel(state *GameState) int {
	if state == nil {
		return 0
	}
	if state.ResultLevel > 0 {
		return state.ResultLevel
	}
	if state.Level > 0 {
		return state.Level
	}
	return 0
}

// IsTargetReached 목표 레벨 도달 여부 확인
func (e *Engine) IsTargetReached(currentLevel int) bool {
	return currentLevel >= e.targetLevel
}

// CanSellItem 판매 가능 여부 확인 (0강이면 판매 불가)
func (e *Engine) CanSellItem(level int) bool {
	return level > 0
}

// LogProfileStatus 프로필 상태 로그 출력 (공통 포맷)
func (e *Engine) LogProfileStatus(profile ProfileCheckResult, modePrefix string) {
	if !profile.OK {
		fmt.Println("📋 프로필 확인 실패 - 새 검으로 시작합니다.")
		return
	}

	fmt.Printf("📋 현재 보유 검: [+%d] %s\n", profile.Level, profile.SwordName)

	if e.IsTargetReached(profile.Level) {
		fmt.Printf("✅ 이미 목표 달성! 현재 +%d (목표: +%d)\n", profile.Level, e.targetLevel)
	} else if profile.Level > 0 {
		fmt.Printf("📈 현재 +%d에서 목표 +%d까지 %s을 시작합니다.\n", profile.Level, e.targetLevel, modePrefix)
	} else {
		fmt.Printf("📈 +0에서 목표 +%d까지 %s을 시작합니다.\n", e.targetLevel, modePrefix)
	}
}

// EnhanceResult 강화 진행 결과
type EnhanceResult struct {
	FinalLevel   int
	Success      bool   // 목표 도달 여부
	Destroyed    bool   // 파괴 여부
	NewSwordName string // 파괴 시 새로 받은 검 이름
	NewSwordType string // 파괴 시 새로 받은 검 타입
}

// EnhanceToTarget 목표 레벨까지 강화 진행 (시작 레벨 지정 가능)
// 기존 enhanceToTargetWithLevel의 개선 버전
func (e *Engine) EnhanceToTarget(itemName string, startLevel int) EnhanceResult {
	currentLevel := startLevel

	for currentLevel < e.targetLevel && e.running {
		if e.checkStop() {
			return EnhanceResult{FinalLevel: currentLevel, Success: false, Destroyed: false}
		}

		// 강화 시도
		e.sendCommand("/강화")
		delay := e.getDelayForLevel(currentLevel)
		time.Sleep(delay)

		// 결과 확인
		text := e.readChatTextWaitForChange(5 * time.Second)
		state := ParseOCRText(text)

		if state == nil {
			continue
		}

		// 파괴 확인
		if state.LastResult == "destroy" {
			result := EnhanceResult{FinalLevel: currentLevel, Success: false, Destroyed: true}

			// 파괴 시 새 검 정보 추출 (판매 결과와 동일한 패턴 사용)
			saleResult := ExtractSaleResult(text)
			if saleResult != nil && saleResult.NewSwordName != "" {
				result.NewSwordName = saleResult.NewSwordName
				result.NewSwordType = DetermineItemType(saleResult.NewSwordName)
			}

			return result
		}

		// 레벨 업데이트 (강화 결과 기반)
		// 핵심: 파싱 실패해도 강화 결과(success/hold)로 레벨 추정
		if state.LastResult == "success" {
			// 강화 성공 = 레벨 +1 (파싱 결과보다 이걸 우선 신뢰)
			currentLevel++
			fmt.Printf("  ⚔️ 강화 성공! +%d 도달\n", currentLevel)
		} else if state.LastResult == "hold" {
			// 유지 = 레벨 변화 없음
			fmt.Printf("  💫 강화 유지 (현재 +%d)\n", currentLevel)
		} else {
			// 결과 불명확 시 파싱된 레벨 사용 (fallback)
			newLevel := e.ExtractCurrentLevel(state)
			if newLevel > currentLevel {
				currentLevel = newLevel
			}
		}

		// 골드 부족 체크
		goldInfo := DetectInsufficientGold(text)
		if goldInfo.IsInsufficient {
			fmt.Printf("⚠️ 골드 부족! 필요: %s, 보유: %s\n",
				FormatGold(goldInfo.RequiredGold), FormatGold(goldInfo.RemainingGold))
			return EnhanceResult{FinalLevel: currentLevel, Success: false, Destroyed: false}
		}
	}

	return EnhanceResult{
		FinalLevel: currentLevel,
		Success:    currentLevel >= e.targetLevel,
		Destroyed:  false,
	}
}

// MeasureGoldProfit 골드 수익 측정 (판매가 - 강화비용이 아닌 순수 판매 수익)
func (e *Engine) MeasureGoldProfit(saleText string, fallbackGold int) (saleGold int, currentGold int) {
	saleResult := ExtractSaleResult(saleText)

	if saleResult != nil && saleResult.SaleGold > 0 {
		return saleResult.SaleGold, saleResult.CurrentGold
	}

	// 폴백: 직접 읽기
	currentGold = e.readCurrentGold()
	return fallbackGold, currentGold
}

// =============================================================================
// 텔레메트리 보고 헬퍼
// =============================================================================

// ReportSwordComplete 검 완료 보고 (loopEnhance, loopSpecial 공통)
func (e *Engine) ReportSwordComplete() {
	e.telem.RecordSword()
	e.telem.TrySend()
}

// ReportGoldMineCycle 골드 채굴 사이클 완료 보고
func (e *Engine) ReportGoldMineCycle(itemName string, level, goldEarned, currentGold int) {
	e.telem.RecordCycle(true)
	e.telem.RecordGold(goldEarned)
	e.telem.RecordSaleWithSword(itemName, level, goldEarned)
	e.telem.RecordGoldChange(currentGold)
	e.telem.TrySend()
}

// ReportCycleFailed 사이클 실패 보고
func (e *Engine) ReportCycleFailed() {
	e.telem.RecordCycle(false)
}

// =============================================================================
// 로그 메시지 헬퍼
// =============================================================================

// LogTargetReached 목표 달성 로그 (공통 포맷)
func (e *Engine) LogTargetReached(itemName string, level int) {
	if itemName != "" {
		fmt.Printf("✅ 이미 목표 달성! [%s] +%d\n", itemName, level)
	} else {
		fmt.Printf("✅ 이미 목표 달성! 현재 +%d (목표: +%d)\n", level, e.targetLevel)
	}
}

// LogEnhanceStart 강화 시작 로그 (공통 포맷)
func (e *Engine) LogEnhanceStart(currentLevel int) {
	if currentLevel > 0 {
		fmt.Printf("📈 현재 +%d에서 목표 +%d까지 강화를 시작합니다.\n", currentLevel, e.targetLevel)
	} else {
		fmt.Printf("📈 +0에서 목표 +%d까지 강화를 시작합니다.\n", e.targetLevel)
	}
}

// LogEnhanceComplete 강화 완료 로그
func (e *Engine) LogEnhanceComplete(itemName string, level int) {
	fmt.Printf("✅ 강화 완료! [%s] +%d\n", itemName, level)
}

// LogEnhanceDestroy 강화 파괴 로그
func (e *Engine) LogEnhanceDestroy(itemName string, level int) {
	fmt.Printf("💥 강화 중 파괴됨 (최종 레벨: +%d)\n", level)
}

// LogSpecialFound 특수 아이템 발견 로그
func (e *Engine) LogSpecialFound(itemName string, level int) {
	fmt.Printf("🎉 특수 아이템 발견! [%s] +%d\n", itemName, level)
}

// LogProfileCheck 프로필 확인 로그
func (e *Engine) LogProfileCheck(profile ProfileCheckResult) {
	if profile.OK {
		fmt.Printf("📋 현재 보유 검: [+%d] %s\n", profile.Level, profile.SwordName)
	} else {
		fmt.Println("📋 프로필 확인 실패 - 새 검으로 시작합니다.")
	}
}

// =============================================================================
// 프로필 분석 출력 헬퍼
// showMyProfile, loopBattle 등에서 공유하는 출력 로직
// =============================================================================

// CheckProfileFull 전체 프로필 정보 조회 (Profile 구조체 반환)
// loopBattle, showMyProfile 등에서 전체 프로필이 필요할 때 사용
func (e *Engine) CheckProfileFull() *Profile {
	// 이전 채팅 기록 초기화하여 새 응답 감지 보장
	e.ResetLastChatText()

	e.sendCommand("/프로필")
	profileText := e.waitForResponse(5 * time.Second)

	if profileText == "" {
		fmt.Println("  ⚠️ 프로필 응답을 받지 못했습니다.")
		return nil
	}

	profile := ParseProfile(profileText)
	if profile == nil || profile.Level < 0 {
		fmt.Printf("  ⚠️ 프로필 파싱 실패. 읽은 텍스트 길이: %d\n", len(profileText))
	}

	return profile
}

// CheckOtherProfile 다른 유저의 프로필 정보 조회
// 카카오톡: Enter 1번 = 줄바꿈, Enter 2번 = 전송
// 1단계: "/프로" + Enter(줄바꿈)
// 2단계: "@유저명" + Enter 2번(전송)
func (e *Engine) CheckOtherProfile(username string) *Profile {
	// 명령어 전송 전 현재 채팅 저장 (새 응답만 감지하기 위해)
	e.SaveLastChatText()

	// 1단계: /프로 + Enter(줄바꿈만)
	e.sendCommandOnce("/프로")

	// 2단계: @유저명 + Enter 2번(전송)
	e.appendAndSend(username)

	// 다른 유저 프로필은 내 이름이 없으므로 필터 없이 읽기
	profileText := e.waitForResponseRaw(3 * time.Second)

	if profileText == "" {
		return nil
	}

	// 해당 유저의 프로필 섹션만 파싱 (다른 유저/본인 프로필 무시)
	return ParseProfileForUser(profileText, username)
}

// PrintEnhanceRateTable 강화 확률표 출력
// fromLevel부터 +20까지의 강화 확률과 예상 판매가를 테이블 형식으로 출력
func PrintEnhanceRateTable(fromLevel int) {
	fmt.Println("📊 강화 확률 (현재 레벨 기준)")
	fmt.Println("   레벨  | 성공  | 유지  | 파괴  | 예상 판매가")
	fmt.Println("   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	rates := GetAllEnhanceRates()
	for lvl := fromLevel; lvl <= 20 && rates != nil && lvl < len(rates); lvl++ {
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
		if lvl == fromLevel {
			marker = "▶ "
		}

		fmt.Printf("   %s+%d→+%d | %4.0f%% | %4.0f%% | %4.0f%% | %s\n",
			marker, lvl, lvl+1, rate.SuccessRate, rate.KeepRate, rate.DestroyRate, priceStr)
	}
	fmt.Println()
}

// PrintTargetSuccessChance 목표 달성 확률 출력
// currentLevel에서 주요 목표 레벨까지의 성공 확률과 예상 시도 횟수 출력
func PrintTargetSuccessChance(currentLevel int) {
	fmt.Println("🎯 목표 달성 확률")
	targets := []int{currentLevel + 1, currentLevel + 2, currentLevel + 3, 10, 12, 15, 20}
	shown := make(map[int]bool)

	for _, target := range targets {
		if target <= currentLevel || target > 20 || shown[target] {
			continue
		}
		shown[target] = true

		chance := CalcEnhanceSuccessChance(currentLevel, target)
		trials := CalcExpectedTrials(currentLevel, target)
		targetPrice := GetSwordPrice(target)

		priceStr := ""
		if targetPrice != nil {
			priceStr = fmt.Sprintf(" (판매가: %sG)", FormatGold(targetPrice.AvgPrice))
		}

		fmt.Printf("   +%d → +%d: %.2f%% (평균 %.0f회 시도)%s\n",
			currentLevel, target, chance, trials, priceStr)
	}
	fmt.Println()
}

// PrintUpsetAnalysis 역배 기대값 분석 출력
// level: 내 레벨, gold: 보유 골드 (배팅 금액 계산용)
func PrintUpsetAnalysis(level, gold int) {
	fmt.Printf("⚡ 역배 분석 (내 레벨: +%d)\n", level)
	fmt.Println("   레벨차 | 승률  | 평균보상 | 기대값")
	fmt.Println("   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	betAmount := 100 // 기본 배팅 금액 가정
	if gold > 0 {
		betAmount = gold / 10 // 보유 골드의 10%를 배팅으로 가정
		if betAmount < 100 {
			betAmount = 100
		}
	}

	for diff := 1; diff <= 3; diff++ {
		reward := GetBattleReward(diff)
		if reward == nil {
			continue
		}

		ev, winRate, avgReward := CalcUpsetExpectedValue(level, level+diff, betAmount)

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
}

// =============================================================================
// 배틀 관련 헬퍼
// =============================================================================

// ReportBattleCycle 배틀 사이클 완료 보고
func (e *Engine) ReportBattleCycle(swordName string, myLevel, targetLevel int, won bool, goldEarned, currentGold int) {
	e.telem.RecordBattleWithSword(swordName, myLevel, targetLevel, won, goldEarned)
	e.telem.RecordGoldChange(currentGold)
	e.telem.TrySend()
}

// PrintBattleStats 배틀 전적 통계 출력
func PrintBattleStats(wins, losses, totalGold int) {
	winRate := float64(0)
	if wins+losses > 0 {
		winRate = float64(wins) / float64(wins+losses) * 100
	}
	fmt.Printf("   📊 전적: %d승 %d패 (%.1f%%) | 수익: %sG\n",
		wins, losses, winRate, FormatGold(totalGold))
}
