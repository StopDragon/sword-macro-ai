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
