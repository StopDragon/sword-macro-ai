package game

import (
	"regexp"
	"strconv"
	"strings"
)

// 검증 상수
const (
	MinLevel = 0
	MaxLevel = 20 // 게임 내 최대 레벨
	MinGold  = 0
	MaxGold  = 1000000000 // 10억 (합리적 최대값)
)

// GameState 게임 상태
type GameState struct {
	Level      int
	Gold       int
	ItemType   string // "trash", "hidden", "none"
	ItemName   string // 아이템 이름 (검, 방망이 등)
	LastResult string // "success", "hold", "destroy", ""
}

// Profile 유저 프로필
type Profile struct {
	Name      string // @유저명
	Level     int    // 현재 검 레벨
	SwordName string // 검 이름
	Wins      int    // 승리 수
	Losses    int    // 패배 수
	Gold      int    // 보유 골드
}

// RankingEntry 랭킹 항목
type RankingEntry struct {
	Rank     int    // 순위
	Username string // @유저명
	Level    int    // 검 레벨
	Wins     int    // 승리 수
	Losses   int    // 패배 수
}

// BattleResult 배틀 결과
type BattleResult struct {
	Winner      string // 승자 유저명
	Loser       string // 패자 유저명
	WinnerLevel int    // 승자 레벨
	LoserLevel  int    // 패자 레벨
	GoldEarned  int    // 획득 골드
	MyName      string // 내 유저명 (비교용)
	Won         bool   // 내가 이겼는지
}

var (
	// 정규식 패턴
	levelPattern   = regexp.MustCompile(`\+(\d+)`)
	goldPattern    = regexp.MustCompile(`(\d{1,3}(?:,\d{3})*)\s*(?:G|골드|gold)`)
	successPattern = regexp.MustCompile(`(?:강화.*성공|레벨.*상승|업그레이드)`)
	holdPattern    = regexp.MustCompile(`(?:강화.*유지|레벨.*유지|실패.*유지)`)
	destroyPattern = regexp.MustCompile(`(?:파괴|부서|사라)`)
	hiddenPattern  = regexp.MustCompile(`(?:히든|hidden|레전더리|전설|유니크)`)
	trashPattern   = regexp.MustCompile(`(?:일반|노말|커먼|쓰레기|낡은)`)
	farmPattern    = regexp.MustCompile(`(?:획득|얻었|드랍|뽑기)`)

	// 판매 관련 패턴
	cantSellPattern   = regexp.MustCompile(`(?:판매할 수 없|가치가 없|팔 수 없)`)
	newSwordPattern   = regexp.MustCompile(`새로운 검.*획득|검.*획득`)

	// 아이템 이름 추출 패턴 (v2)
	hiddenNamePattern = regexp.MustCompile(`(?:히든|hidden).*?『([^』]+)』`)
	swordNamePattern  = regexp.MustCompile(`\[([^\]]+)\]\s*(.+?)(?:\s|$|』)`)
	// 파밍 결과에서 아이템 이름 추출: "불꽃검 획득!" "방망이를 얻었습니다"
	farmItemPattern   = regexp.MustCompile(`『?([^『』\[\]]+?)』?\s*(?:획득|얻|드랍|뽑)`)
	// 괄호 안 아이템: 『용검』, 『불꽃검』
	bracketItemPattern = regexp.MustCompile(`『([^』]+)』`)

	// 프로필 패턴
	profileNamePattern   = regexp.MustCompile(`이름:\s*(@\S+)`)
	profileWinsPattern   = regexp.MustCompile(`(\d+)승`)
	profileLossesPattern = regexp.MustCompile(`(\d+)패`)
	profileGoldPattern   = regexp.MustCompile(`보유\s*골드:\s*(\d{1,3}(?:,\d{3})*)\s*G`)
	profileSwordPattern  = regexp.MustCompile(`보유\s*검:\s*\[([^\]]+)\]\s*(.+)`)

	// 랭킹 패턴
	rankingEntryPattern = regexp.MustCompile(`(\d+)위:\s*(@\S+)?\s*\(\[?\+?(\d+)\]?`)
	rankingBattlePattern = regexp.MustCompile(`(\d+)위:\s*(@\S+)?\s*\((\d+)승\s*(\d+)패\)`)

	// 배틀 결과 패턴
	battleResultPattern = regexp.MustCompile(`결과.*(@\S+).*승리`)
	battleGoldPattern   = regexp.MustCompile(`전리품\s*(\d{1,3}(?:,\d{3})*)\s*G`)
	battleVsPattern     = regexp.MustCompile(`(@\S+)\s*『\[([^\]]+)\]`)
)

// ParseOCRText OCR 텍스트 파싱 (범위 검증 포함)
func ParseOCRText(text string) *GameState {
	state := &GameState{
		Level:    -1,
		Gold:     -1,
		ItemType: "none",
	}

	text = strings.ToLower(text)
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 레벨 파싱 (범위 검증 포함)
		if matches := levelPattern.FindStringSubmatch(line); len(matches) > 1 {
			if level, err := strconv.Atoi(matches[1]); err == nil {
				if ValidateLevel(level) {
					state.Level = level
				}
			}
		}

		// 골드 파싱 (범위 검증 포함)
		if matches := goldPattern.FindStringSubmatch(line); len(matches) > 1 {
			goldStr := strings.ReplaceAll(matches[1], ",", "")
			if gold, err := strconv.Atoi(goldStr); err == nil {
				if ValidateGold(gold) {
					state.Gold = gold
				}
			}
		}

		// 강화 결과 파싱
		if successPattern.MatchString(line) {
			state.LastResult = "success"
		} else if destroyPattern.MatchString(line) {
			state.LastResult = "destroy"
		} else if holdPattern.MatchString(line) {
			state.LastResult = "hold"
		}

		// 아이템 타입 및 이름 파싱
		if farmPattern.MatchString(line) {
			if hiddenPattern.MatchString(line) {
				state.ItemType = "hidden"
			} else if trashPattern.MatchString(line) {
				state.ItemType = "trash"
			} else {
				state.ItemType = "normal" // 일반 아이템
			}

			// 아이템 이름 추출 시도
			if state.ItemName == "" {
				state.ItemName = ExtractItemName(line)
			}
		}
	}

	return state
}

// DetectEnhanceResult 강화 결과 감지
func DetectEnhanceResult(text string) string {
	text = strings.ToLower(text)

	if successPattern.MatchString(text) {
		return "success"
	}
	if destroyPattern.MatchString(text) {
		return "destroy"
	}
	if holdPattern.MatchString(text) {
		return "hold"
	}

	return ""
}

// DetectItemType 아이템 타입 감지
func DetectItemType(text string) string {
	text = strings.ToLower(text)

	if hiddenPattern.MatchString(text) {
		return "hidden"
	}
	if trashPattern.MatchString(text) {
		return "trash"
	}

	return "unknown"
}

// CannotSell 판매 불가 메시지 감지 (0강 아이템)
func CannotSell(text string) bool {
	return cantSellPattern.MatchString(strings.ToLower(text))
}

// GotNewSword 새 검 획득 메시지 감지
func GotNewSword(text string) bool {
	return newSwordPattern.MatchString(strings.ToLower(text))
}

// ExtractLevel 레벨 추출 (범위 검증 포함)
func ExtractLevel(text string) int {
	if matches := levelPattern.FindStringSubmatch(text); len(matches) > 1 {
		if level, err := strconv.Atoi(matches[1]); err == nil {
			// 범위 검증
			if level >= MinLevel && level <= MaxLevel {
				return level
			}
		}
	}
	return -1
}

// ExtractGold 골드 추출 (범위 검증 포함)
func ExtractGold(text string) int {
	if matches := goldPattern.FindStringSubmatch(text); len(matches) > 1 {
		goldStr := strings.ReplaceAll(matches[1], ",", "")
		if gold, err := strconv.Atoi(goldStr); err == nil {
			// 범위 검증
			if gold >= MinGold && gold <= MaxGold {
				return gold
			}
		}
	}
	return -1
}

// ValidateLevel 레벨 범위 검증
func ValidateLevel(level int) bool {
	return level >= MinLevel && level <= MaxLevel
}

// ValidateGold 골드 범위 검증
func ValidateGold(gold int) bool {
	return gold >= MinGold && gold <= MaxGold
}

// ParseProfile 프로필 파싱
// /프로필 명령어 결과에서 프로필 정보 추출
func ParseProfile(text string) *Profile {
	profile := &Profile{
		Level: -1,
		Gold:  -1,
	}

	// 이름 추출
	if matches := profileNamePattern.FindStringSubmatch(text); len(matches) > 1 {
		profile.Name = matches[1]
	}

	// 전적 추출
	if matches := profileWinsPattern.FindStringSubmatch(text); len(matches) > 1 {
		if wins, err := strconv.Atoi(matches[1]); err == nil {
			profile.Wins = wins
		}
	}
	if matches := profileLossesPattern.FindStringSubmatch(text); len(matches) > 1 {
		if losses, err := strconv.Atoi(matches[1]); err == nil {
			profile.Losses = losses
		}
	}

	// 골드 추출
	if matches := profileGoldPattern.FindStringSubmatch(text); len(matches) > 1 {
		goldStr := strings.ReplaceAll(matches[1], ",", "")
		if gold, err := strconv.Atoi(goldStr); err == nil {
			profile.Gold = gold
		}
	}

	// 보유 검 추출 (레벨 + 이름)
	if matches := profileSwordPattern.FindStringSubmatch(text); len(matches) > 2 {
		levelStr := strings.TrimPrefix(matches[1], "+")
		if level, err := strconv.Atoi(levelStr); err == nil {
			profile.Level = level
		}
		profile.SwordName = strings.TrimSpace(matches[2])
	}

	// 레벨이 없으면 일반 패턴으로 시도
	if profile.Level == -1 {
		profile.Level = ExtractLevel(text)
	}

	return profile
}

// ParseRanking 랭킹 파싱
// /랭킹 명령어 결과에서 강화 랭킹 정보 추출
func ParseRanking(text string) []RankingEntry {
	var entries []RankingEntry
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		entry := RankingEntry{}

		// 강화 랭킹 패턴 (1위: @유저 ([+20] 검이름))
		if matches := rankingEntryPattern.FindStringSubmatch(line); len(matches) > 3 {
			if rank, err := strconv.Atoi(matches[1]); err == nil {
				entry.Rank = rank
			}
			entry.Username = matches[2] // @유저명 또는 빈 문자열
			if level, err := strconv.Atoi(matches[3]); err == nil {
				entry.Level = level
			}
			if entry.Level > 0 {
				entries = append(entries, entry)
			}
			continue
		}

		// 배틀 랭킹 패턴 (1위: @유저 (2255승 838패))
		if matches := rankingBattlePattern.FindStringSubmatch(line); len(matches) > 4 {
			if rank, err := strconv.Atoi(matches[1]); err == nil {
				entry.Rank = rank
			}
			entry.Username = matches[2]
			if wins, err := strconv.Atoi(matches[3]); err == nil {
				entry.Wins = wins
			}
			if losses, err := strconv.Atoi(matches[4]); err == nil {
				entry.Losses = losses
			}
			entries = append(entries, entry)
		}
	}

	return entries
}

// ParseBattleResult 배틀 결과 파싱
func ParseBattleResult(text string, myName string) *BattleResult {
	result := &BattleResult{
		MyName:      myName,
		WinnerLevel: -1,
		LoserLevel:  -1,
		GoldEarned:  0,
	}

	// 승자 추출
	if matches := battleResultPattern.FindStringSubmatch(text); len(matches) > 1 {
		result.Winner = matches[1]
		result.Won = (result.Winner == myName)
	}

	// 획득 골드 추출
	if matches := battleGoldPattern.FindStringSubmatch(text); len(matches) > 1 {
		goldStr := strings.ReplaceAll(matches[1], ",", "")
		if gold, err := strconv.Atoi(goldStr); err == nil {
			result.GoldEarned = gold
		}
	}

	// VS 패턴에서 양측 정보 추출
	vsMatches := battleVsPattern.FindAllStringSubmatch(text, 2)
	if len(vsMatches) >= 2 {
		// 첫 번째 참가자
		user1 := vsMatches[0][1]
		level1 := ExtractLevel(vsMatches[0][2])

		// 두 번째 참가자
		user2 := vsMatches[1][1]
		level2 := ExtractLevel(vsMatches[1][2])

		if result.Winner == user1 {
			result.WinnerLevel = level1
			result.Loser = user2
			result.LoserLevel = level2
		} else if result.Winner == user2 {
			result.WinnerLevel = level2
			result.Loser = user1
			result.LoserLevel = level1
		}
	}

	return result
}

// FindTargetsInRanking 랭킹에서 역배 타겟 찾기
func FindTargetsInRanking(entries []RankingEntry, myLevel int, levelDiff int) []RankingEntry {
	var targets []RankingEntry

	minTarget := myLevel + 1
	maxTarget := myLevel + levelDiff

	for _, entry := range entries {
		if entry.Level >= minTarget && entry.Level <= maxTarget && entry.Username != "" {
			targets = append(targets, entry)
		}
	}

	return targets
}

// === v2 새로운 추출 함수들 ===

// ExtractHiddenName 히든 검 이름 추출
// 예: "히든 검 『용검』 획득!" -> "용검"
func ExtractHiddenName(text string) string {
	if matches := hiddenNamePattern.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractSwordName 검 이름 추출 (프로필, 배틀 결과 등에서)
// 예: "[+10] 불꽃검" -> "불꽃검"
// 예: "『[+10] 불꽃검』" -> "불꽃검"
func ExtractSwordName(text string) string {
	// 먼저 레벨 패턴 [+10] 을 찾고 그 뒤의 텍스트를 추출
	if matches := swordNamePattern.FindStringSubmatch(text); len(matches) > 2 {
		name := strings.TrimSpace(matches[2])
		if name != "" {
			return name
		}
	}

	// 대안: 『』 괄호 안에서 검 이름 추출
	bracketPattern := regexp.MustCompile(`『([^』]+)』`)
	if matches := bracketPattern.FindStringSubmatch(text); len(matches) > 1 {
		innerText := matches[1]
		// [+N] 패턴 제거하고 검 이름만 추출
		swordOnly := regexp.MustCompile(`\[\+?\d+\]\s*`).ReplaceAllString(innerText, "")
		return strings.TrimSpace(swordOnly)
	}

	return ""
}

// ExtractSwordInfo 검 레벨과 이름 동시 추출
// 예: "[+10] 불꽃검" -> (10, "불꽃검")
func ExtractSwordInfo(text string) (int, string) {
	level := ExtractLevel(text)
	name := ExtractSwordName(text)
	return level, name
}

// ExtractItemName 아이템 이름 추출 (모든 종류: 검, 방망이, 도끼 등)
// 파밍 결과 메시지에서 아이템 이름을 추출
// 예: "『불꽃검』 획득!" -> "불꽃검"
// 예: "방망이를 얻었습니다" -> "방망이"
// 예: "히든 아이템 『용검』 발견!" -> "용검"
func ExtractItemName(text string) string {
	// 1순위: 히든 아이템 패턴
	if matches := hiddenNamePattern.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// 2순위: 『』 괄호 안의 아이템
	if matches := bracketItemPattern.FindStringSubmatch(text); len(matches) > 1 {
		innerText := matches[1]
		// [+N] 패턴이 있으면 제거
		swordOnly := regexp.MustCompile(`\[\+?\d+\]\s*`).ReplaceAllString(innerText, "")
		name := strings.TrimSpace(swordOnly)
		if name != "" {
			return name
		}
	}

	// 3순위: "XXX 획득/얻/드랍" 패턴
	if matches := farmItemPattern.FindStringSubmatch(text); len(matches) > 1 {
		name := strings.TrimSpace(matches[1])
		// 불필요한 접미사 제거
		name = strings.TrimSuffix(name, "을")
		name = strings.TrimSuffix(name, "를")
		name = strings.TrimSuffix(name, "이")
		name = strings.TrimSuffix(name, "가")
		if name != "" && len(name) < 20 { // 너무 긴 문자열 제외
			return name
		}
	}

	return ""
}

// ExtractItemInfo 아이템 정보 전체 추출 (레벨, 이름, 타입)
type ItemInfo struct {
	Name  string // 아이템 이름
	Level int    // 레벨 (-1 if 없음)
	Type  string // "hidden", "normal", "trash", "unknown"
}

// ExtractFullItemInfo 파밍 결과에서 아이템 정보 전체 추출
func ExtractFullItemInfo(text string) *ItemInfo {
	info := &ItemInfo{
		Level: -1,
		Type:  "unknown",
	}

	// 아이템 이름 추출
	info.Name = ExtractItemName(text)

	// 레벨 추출 (있으면)
	info.Level = ExtractLevel(text)

	// 타입 결정
	if hiddenPattern.MatchString(strings.ToLower(text)) {
		info.Type = "hidden"
	} else if trashPattern.MatchString(strings.ToLower(text)) {
		info.Type = "trash"
	} else if info.Name != "" {
		info.Type = "normal"
	}

	return info
}
