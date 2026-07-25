package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type AggregationMode string

const (
	AggMax AggregationMode = "max"
	AggAvg AggregationMode = "avg"
)

type CurvePoint struct {
	Temp float64 `yaml:"temp" json:"temp"`
	PWM  int     `yaml:"pwm" json:"pwm"`
}

type FanRule struct {
	FanName        string          `yaml:"fan_name" json:"fan_name"`
	PWMID          string          `yaml:"pwm_id" json:"pwm_id"`
	Mode           string          `yaml:"mode" json:"mode"` // "auto" or "manual"
	ManualPWM      int             `yaml:"manual_pwm" json:"manual_pwm"`
	SensorIDs      []string        `yaml:"sensor_ids" json:"sensor_ids"`
	SensorAggMode  AggregationMode `yaml:"sensor_agg_mode" json:"sensor_agg_mode"` // "max" or "avg"
	MinPWM         int             `yaml:"min_pwm" json:"min_pwm"`
	MaxPWM         int             `yaml:"max_pwm" json:"max_pwm"`
	Curve          []CurvePoint    `yaml:"curve" json:"curve"`
	HysteresisTemp float64         `yaml:"hysteresis_temp" json:"hysteresis_temp"`
}

type Config struct {
	PollIntervalSec int       `yaml:"poll_interval_sec" json:"poll_interval_sec"`
	APIPort         int       `yaml:"api_port" json:"api_port"`
	Debug           bool      `yaml:"debug" json:"debug"`
	Rules           []FanRule `yaml:"rules" json:"rules"`
}

func DefaultConfig() *Config {
	return &Config{
		PollIntervalSec: 2,
		APIPort:         8080,
		Debug:           false,
		Rules:           []FanRule{},
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
