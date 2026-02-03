package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const ConfigFile = "sword_config.json"

// Config 매크로 설정
type Config struct {
	// 좌표 설정
	ClickX int `json:"click_x"`
	ClickY int `json:"click_y"`
	LockXY bool `json:"lock_xy"`

	// 딜레이 설정 (초)
	TrashDelay    float64 `json:"trash_delay"`
	LowDelay      float64 `json:"low_delay"`
	MidDelay      float64 `json:"mid_delay"`
	HighDelay     float64 `json:"high_delay"`
	SlowdownLevel int     `json:"slowdown_level"`

	// 게임 설정
	GoldMineTarget int `json:"gold_mine_target"`
	MinGold        int `json:"min_gold"`

	// 배틀 설정
	BattleLevelDiff int     `json:"battle_level_diff"` // 역배 레벨 차이 (1-3)
	BattleCooldown  float64 `json:"battle_cooldown"`   // 배틀 간 쿨다운 (초)
	BattleMinGold   int     `json:"battle_min_gold"`   // 최소 보유 골드 (이하면 중단)

	// 클립보드 텍스트 읽기
	ChatOffsetY int `json:"chat_offset_y"` // 입력창에서 채팅 영역까지 거리 (픽셀)

	// 오버레이 설정
	OverlayChatWidth  int `json:"overlay_chat_width"`  // 채팅 영역 너비
	OverlayChatHeight int `json:"overlay_chat_height"` // 채팅 영역 높이
	OverlayInputWidth int `json:"overlay_input_width"` // 입력 영역 너비
	OverlayInputHeight int `json:"overlay_input_height"` // 입력 영역 높이
}

// Default 기본 설정 반환
func Default() *Config {
	return &Config{
		ClickX:          0,
		ClickY:          0,
		LockXY:          false,
		TrashDelay:      1.2,
		LowDelay:        1.5,
		MidDelay:        2.5,
		HighDelay:       3.5,
		SlowdownLevel:   9,
		GoldMineTarget:  10,
		MinGold:         0,
		BattleLevelDiff: 2,
		BattleCooldown:  5.0,
		BattleMinGold:   1000,
		ChatOffsetY:     40, // 입력창 클릭 좌표에서 40픽셀 위 (채팅 영역 클릭용)
		// 오버레이 기본값
		OverlayChatWidth:   380,
		OverlayChatHeight:  430,
		OverlayInputWidth:  380,
		OverlayInputHeight: 50,
	}
}

// Load 설정 파일 로드
func Load() (*Config, error) {
	configPath := getConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}

	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save 설정 파일 저장
func (c *Config) Save() error {
	configPath := getConfigPath()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func getConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ConfigFile
	}
	return filepath.Join(filepath.Dir(exe), ConfigFile)
}
