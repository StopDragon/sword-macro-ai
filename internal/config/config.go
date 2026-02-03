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

	// 캡처 영역
	CaptureW int `json:"capture_w"`
	CaptureH int `json:"capture_h"`
	InputBoxH int `json:"input_box_h"`
}

// Default 기본 설정 반환
func Default() *Config {
	return &Config{
		ClickX:        0,
		ClickY:        0,
		LockXY:        false,
		TrashDelay:    1.2,
		LowDelay:      1.5,
		MidDelay:      2.5,
		HighDelay:     3.5,
		SlowdownLevel: 9,
		GoldMineTarget: 10,
		MinGold:       0,
		CaptureW:      375,
		CaptureH:      550,
		InputBoxH:     80,
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
