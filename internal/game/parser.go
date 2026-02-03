package game

import (
	"regexp"
	"strconv"
	"strings"
)

// GameState 게임 상태
type GameState struct {
	Level      int
	Gold       int
	ItemType   string // "trash", "hidden", "none"
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
	trashPattern   = regexp.MustCompile(`(?:일반|노말|커먼|쓰레기)`)
	farmPattern    = regexp.MustCompile(`(?:획득|얻었|드랍|뽑기)`)

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

// ParseOCRText OCR 텍스트 파싱
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

		// 레벨 파싱
		if matches := levelPattern.FindStringSubmatch(line); len(matches) > 1 {
			if level, err := strconv.Atoi(matches[1]); err == nil {
				state.Level = level
			}
		}

		// 골드 파싱
		if matches := goldPattern.FindStringSubmatch(line); len(matches) > 1 {
			goldStr := strings.ReplaceAll(matches[1], ",", "")
			if gold, err := strconv.Atoi(goldStr); err == nil {
				state.Gold = gold
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

		// 아이템 타입 파싱
		if farmPattern.MatchString(line) {
			if hiddenPattern.MatchString(line) {
				state.ItemType = "hidden"
			} else if trashPattern.MatchString(line) {
				state.ItemType = "trash"
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

// ExtractLevel 레벨 추출
func ExtractLevel(text string) int {
	if matches := levelPattern.FindStringSubmatch(text); len(matches) > 1 {
		if level, err := strconv.Atoi(matches[1]); err == nil {
			return level
		}
	}
	return -1
}

// ExtractGold 골드 추출
func ExtractGold(text string) int {
	if matches := goldPattern.FindStringSubmatch(text); len(matches) > 1 {
		goldStr := strings.ReplaceAll(matches[1], ",", "")
		if gold, err := strconv.Atoi(goldStr); err == nil {
			return gold
		}
	}
	return -1
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
