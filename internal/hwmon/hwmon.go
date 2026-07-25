package hwmon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type TempSensor struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	InputPath string  `json:"-"`
}

type FanSensor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RPM       int    `json:"rpm"`
	InputPath string `json:"-"`
}

type PWMControl struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Value      int    `json:"value"`
	Enable     int    `json:"enable"` // 0: off, 1: manual, 2+: automatic curves
	EnablePath string `json:"-"`
	PWMPath    string `json:"-"`
}

type Telemetry struct {
	Temps []TempSensor `json:"temperatures"`
	Fans  []FanSensor  `json:"fans"`
	PWMs  []PWMControl `json:"pwms"`
}

type Scanner struct {
	SysPath string
}

func NewScanner() *Scanner {
	return &Scanner{SysPath: "/sys/class/hwmon"}
}

func (s *Scanner) GetTelemetry() (Telemetry, error) {
	temps, fans, pwms, err := s.DiscoverSensingDevices()
	if err != nil {
		return Telemetry{}, err
	}
	return Telemetry{
		Temps: temps,
		Fans:  fans,
		PWMs:  pwms,
	}, nil
}

func (s *Scanner) DiscoverSensingDevices() ([]TempSensor, []FanSensor, []PWMControl, error) {
	var temps []TempSensor
	var fans []FanSensor
	var pwms []PWMControl

	entries, err := os.ReadDir(s.SysPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read sysfs hwmon path: %w", err)
	}

	for _, entry := range entries {
		hwmonDir := filepath.Join(s.SysPath, entry.Name())
		hwmonName := s.readString(filepath.Join(hwmonDir, "name"))

		files, err := os.ReadDir(hwmonDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			name := f.Name()

			if strings.HasPrefix(name, "temp") && strings.HasSuffix(name, "_input") {
				prefix := strings.TrimSuffix(name, "_input")
				label := s.readString(filepath.Join(hwmonDir, prefix+"_label"))
				if label == "" {
					label = prefix
				}
				val := s.readFloat(filepath.Join(hwmonDir, name)) / 1000.0

				temps = append(temps, TempSensor{
					ID:        fmt.Sprintf("%s_%s", entry.Name(), prefix),
					Name:      fmt.Sprintf("%s (%s)", hwmonName, label),
					Value:     val,
					InputPath: filepath.Join(hwmonDir, name),
				})
			}

			if strings.HasPrefix(name, "fan") && strings.HasSuffix(name, "_input") {
				prefix := strings.TrimSuffix(name, "_input")
				val := s.readInt(filepath.Join(hwmonDir, name))

				fans = append(fans, FanSensor{
					ID:        fmt.Sprintf("%s_%s", entry.Name(), prefix),
					Name:      fmt.Sprintf("%s (%s)", hwmonName, prefix),
					RPM:       val,
					InputPath: filepath.Join(hwmonDir, name),
				})
			}

			if strings.HasPrefix(name, "pwm") && !strings.Contains(name, "_") {
				val := s.readInt(filepath.Join(hwmonDir, name))
				enableVal := 1
				enablePath := filepath.Join(hwmonDir, name+"_enable")
				if _, err := os.Stat(enablePath); err == nil {
					enableVal = s.readInt(enablePath)
				}

				pwms = append(pwms, PWMControl{
					ID:         fmt.Sprintf("%s_%s", entry.Name(), name),
					Name:       fmt.Sprintf("%s (%s)", hwmonName, name),
					Value:      val,
					Enable:     enableVal,
					EnablePath: enablePath,
					PWMPath:    filepath.Join(hwmonDir, name),
				})
			}
		}
	}

	return temps, fans, pwms, nil
}

func (s *Scanner) ReadRPM(path string) int {
	return s.readInt(path)
}

func (s *Scanner) SetPWM(pwm PWMControl, value int) error {
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}

	if pwm.EnablePath != "" {
		_ = os.WriteFile(pwm.EnablePath, []byte("1"), 0644)
	}

	valStr := strconv.Itoa(value)
	return os.WriteFile(pwm.PWMPath, []byte(valStr), 0644)
}

func (s *Scanner) RestorePWMState(pwm PWMControl, origValue int, origEnable int) {
	if pwm.EnablePath != "" {
		_ = os.WriteFile(pwm.EnablePath, []byte(strconv.Itoa(origEnable)), 0644)
	}
	_ = os.WriteFile(pwm.PWMPath, []byte(strconv.Itoa(origValue)), 0644)
}

func (s *Scanner) readString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Scanner) readInt(path string) int {
	str := s.readString(path)
	v, _ := strconv.Atoi(str)
	return v
}

func (s *Scanner) readFloat(path string) float64 {
	str := s.readString(path)
	v, _ := strconv.ParseFloat(str, 64)
	return v
}
