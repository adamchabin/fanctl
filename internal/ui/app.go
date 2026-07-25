package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"fanctl/internal/config"
	"fanctl/internal/hwmon"
)

type Step int

const (
	StepSelectFan Step = iota
	StepConfirmReset
	StepAutoDetect
	StepAdjustLimits
	StepSelectSensors
	StepSummary
)

type autoDetectResultMsg struct {
	matchedPWM hwmon.PWMControl
	matched    bool
	err        error
}

type detectProgressMsg string

type tickMsg time.Time

type keyMap struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Enter key.Binding
	Space key.Binding
	Tab   key.Binding
	Back  key.Binding
	Quit  key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select/confirm"),
		),
		Space: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle selection"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch focus"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "backspace"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

type Model struct {
	scanner   *hwmon.Scanner
	cfg       *config.Config
	apiPort   int
	keys      keyMap
	telemetry hwmon.Telemetry

	currentStep Step

	selectedFanIdx   int
	selectedTempIdx  int
	selectedTempIdxs map[int]bool

	confirmResetYes bool

	isDetecting   bool
	detectStatus  string
	detectErr     error
	matchedPWM    *hwmon.PWMControl
	calibMinPWM   int
	calibMaxPWM   int
	calibFocusMin int
	calibAggMode  string

	detectChan chan tea.Msg

	width  int
	height int
}

func NewModel(scanner *hwmon.Scanner, apiPort int) Model {
	t, _ := scanner.GetTelemetry()
	cfg := fetchRemoteConfig(apiPort)
	return Model{
		scanner:          scanner,
		cfg:              cfg,
		apiPort:          apiPort,
		keys:             defaultKeyMap(),
		telemetry:        t,
		currentStep:      StepSelectFan,
		selectedFanIdx:   0,
		selectedTempIdx:  0,
		selectedTempIdxs: make(map[int]bool),
		confirmResetYes:  false,
		calibMinPWM:      35,
		calibMaxPWM:      255,
		calibFocusMin:    1,
		calibAggMode:     "max",
	}
}

func fetchRemoteConfig(apiPort int) *config.Config {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/config", apiPort))
	if err != nil {
		return config.DefaultConfig()
	}
	defer resp.Body.Close()

	var cfg config.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return config.DefaultConfig()
	}
	return &cfg
}

func saveRemoteConfig(apiPort int, cfg *config.Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("http://127.0.0.1:%d/api/v1/config", apiPort), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to save config via API, status: %d", resp.StatusCode)
	}
	return nil
}

func (m Model) Init() tea.Cmd {
	return m.tickCmd()
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) listenDetectChan() tea.Cmd {
	return func() tea.Msg {
		if m.detectChan == nil {
			return nil
		}
		msg, ok := <-m.detectChan
		if !ok {
			return nil
		}
		return msg
	}
}

func (m Model) findRuleForFan(fanIdx int) *config.FanRule {
	if fanIdx >= len(m.telemetry.Fans) {
		return nil
	}
	fan := m.telemetry.Fans[fanIdx]
	for i := range m.cfg.Rules {
		if i == fanIdx && m.cfg.Rules[i].PWMID != "" {
			return &m.cfg.Rules[i]
		}
	}
	if fanIdx < len(m.cfg.Rules) {
		return &m.cfg.Rules[fanIdx]
	}
	_ = fan
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		if t, err := m.scanner.GetTelemetry(); err == nil {
			m.telemetry = t
		}
		if freshCfg := fetchRemoteConfig(m.apiPort); freshCfg != nil {
			m.cfg = freshCfg
		}
		cmds = append(cmds, m.tickCmd())

	case detectProgressMsg:
		m.detectStatus = string(msg)
		cmds = append(cmds, m.listenDetectChan())

	case autoDetectResultMsg:
		m.isDetecting = false
		m.detectChan = nil
		if msg.err != nil {
			m.detectErr = msg.err
		} else if msg.matched {
			m.matchedPWM = &msg.matchedPWM
			m.currentStep = StepAdjustLimits
			m.applyCurrentLivePWM()
		}

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}

		switch m.currentStep {
		case StepSelectFan:
			switch {
			case key.Matches(msg, m.keys.Up):
				if m.selectedFanIdx > 0 {
					m.selectedFanIdx--
				}
			case key.Matches(msg, m.keys.Down):
				if m.selectedFanIdx < len(m.telemetry.Fans)-1 {
					m.selectedFanIdx++
				}
			case key.Matches(msg, m.keys.Enter):
				if len(m.telemetry.Fans) > 0 {
					rule := m.findRuleForFan(m.selectedFanIdx)
					if rule != nil {
						m.confirmResetYes = false
						m.currentStep = StepConfirmReset
					} else {
						m.startDetectionWorkflow()
						cmds = append(cmds, m.listenDetectChan())
					}
				}
			}

		case StepConfirmReset:
			switch {
			case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Tab):
				m.confirmResetYes = !m.confirmResetYes
			case key.Matches(msg, m.keys.Enter):
				if m.confirmResetYes {
					m.removeRuleForFan(m.selectedFanIdx)
					m.startDetectionWorkflow()
					cmds = append(cmds, m.listenDetectChan())
				} else {
					m.resetToFanSelection()
				}
			case key.Matches(msg, m.keys.Back):
				m.resetToFanSelection()
			}

		case StepAutoDetect:
			if key.Matches(msg, m.keys.Back) && !m.isDetecting {
				m.resetToFanSelection()
			}

		case StepAdjustLimits:
			switch {
			case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Down):
				if m.calibFocusMin == 1 {
					m.calibFocusMin = 0
				} else {
					m.calibFocusMin = 1
				}
				m.applyCurrentLivePWM()

			case key.Matches(msg, m.keys.Left):
				if m.calibFocusMin == 1 {
					if m.calibMinPWM-5 >= 0 {
						m.calibMinPWM -= 5
					}
				} else {
					if m.calibMaxPWM-5 >= m.calibMinPWM {
						m.calibMaxPWM -= 5
					}
				}
				m.applyCurrentLivePWM()

			case key.Matches(msg, m.keys.Right):
				if m.calibFocusMin == 1 {
					if m.calibMinPWM+5 <= m.calibMaxPWM {
						m.calibMinPWM += 5
					}
				} else {
					if m.calibMaxPWM+5 <= 255 {
						m.calibMaxPWM += 5
					}
				}
				m.applyCurrentLivePWM()

			case key.Matches(msg, m.keys.Enter):
				m.selectedTempIdx = 0
				m.selectedTempIdxs = make(map[int]bool)
				m.currentStep = StepSelectSensors

			case key.Matches(msg, m.keys.Back):
				m.resetToFanSelection()
			}

		case StepSelectSensors:
			switch {
			case key.Matches(msg, m.keys.Up):
				if m.selectedTempIdx > 0 {
					m.selectedTempIdx--
				}
			case key.Matches(msg, m.keys.Down):
				if m.selectedTempIdx < len(m.telemetry.Temps)-1 {
					m.selectedTempIdx++
				}
			case key.Matches(msg, m.keys.Space):
				m.selectedTempIdxs[m.selectedTempIdx] = !m.selectedTempIdxs[m.selectedTempIdx]
			case key.Matches(msg, m.keys.Enter):
				m.saveRule()
				m.currentStep = StepSummary
			case key.Matches(msg, m.keys.Back):
				m.currentStep = StepAdjustLimits
				m.applyCurrentLivePWM()
			}

		case StepSummary:
			if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.Back) {
				m.resetToFanSelection()
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) startDetectionWorkflow() {
	m.currentStep = StepAutoDetect
	m.isDetecting = true
	m.detectErr = nil
	m.detectStatus = "Initializing auto-detection..."
	m.detectChan = make(chan tea.Msg, 20)

	go m.startAutoDetectWorker(m.telemetry.Fans[m.selectedFanIdx], m.detectChan)
}

func (m *Model) removeRuleForFan(fanIdx int) {
	if fanIdx >= len(m.telemetry.Fans) {
		return
	}
	var newRules []config.FanRule
	for i, r := range m.cfg.Rules {
		if i != fanIdx {
			newRules = append(newRules, r)
		}
	}
	m.cfg.Rules = newRules
	_ = saveRemoteConfig(m.apiPort, m.cfg)
}

func (m *Model) applyCurrentLivePWM() {
	if m.matchedPWM == nil {
		return
	}
	targetPWM := m.calibMinPWM
	if m.calibFocusMin == 0 {
		targetPWM = m.calibMaxPWM
	}
	_ = m.scanner.SetPWM(*m.matchedPWM, targetPWM)
}

func (m *Model) resetToFanSelection() {
	m.currentStep = StepSelectFan
	m.selectedTempIdx = 0
	m.selectedTempIdxs = make(map[int]bool)
	m.confirmResetYes = false
	m.calibMinPWM = 35
	m.calibMaxPWM = 255
	m.calibFocusMin = 1
	m.matchedPWM = nil
	m.detectStatus = ""
	m.detectChan = nil
	if m.selectedFanIdx >= len(m.telemetry.Fans) {
		m.selectedFanIdx = 0
	}
}

func (m Model) startAutoDetectWorker(targetFan hwmon.FanSensor, ch chan tea.Msg) {
	defer close(ch)

	var debugLogs string
	stepPWM := 25

	assignedPWMs := make(map[string]bool)
	if m.cfg != nil {
		for _, rule := range m.cfg.Rules {
			if rule.PWMID != "" {
				assignedPWMs[rule.PWMID] = true
			}
		}
	}

	for i := range m.telemetry.PWMs {
		pwm := m.telemetry.PWMs[i]

		if assignedPWMs[pwm.ID] {
			debugLogs += fmt.Sprintf("[%s: already assigned, skipped] ", pwm.Name)
			continue
		}

		originalPWM := pwm.Value
		originalEnable := pwm.Enable

		ch <- detectProgressMsg(fmt.Sprintf("Testing channel '%s' (PWM: 255/255)...", pwm.Name))

		_ = m.scanner.SetPWM(pwm, 255)
		time.Sleep(3 * time.Second)

		maxRPM := m.scanner.ReadRPM(targetFan.InputPath)
		if maxRPM <= 0 {
			m.scanner.RestorePWMState(pwm, originalPWM, originalEnable)
			debugLogs += fmt.Sprintf("[%s: fan RPM is 0] ", pwm.Name)
			continue
		}

		currentPWM := 255
		detected := false

		for currentPWM > stepPWM {
			currentPWM -= stepPWM

			currentRPM := m.scanner.ReadRPM(targetFan.InputPath)
			ch <- detectProgressMsg(fmt.Sprintf("Testing '%s': setting PWM to %d/255 (Current speed: %d RPM)...", pwm.Name, currentPWM, currentRPM))

			_ = m.scanner.SetPWM(pwm, currentPWM)
			time.Sleep(3 * time.Second)

			currentRPM = m.scanner.ReadRPM(targetFan.InputPath)
			dropFromMax := maxRPM - currentRPM

			if dropFromMax > 250 || (maxRPM > 0 && float64(dropFromMax)/float64(maxRPM) > 0.10) {
				detected = true
				break
			}
		}

		if detected {
			time.Sleep(1 * time.Second)
			ch <- autoDetectResultMsg{
				matchedPWM: pwm,
				matched:    true,
				err:        nil,
			}
			return
		}

		m.scanner.RestorePWMState(pwm, originalPWM, originalEnable)

		lastRPM := m.scanner.ReadRPM(targetFan.InputPath)
		debugLogs += fmt.Sprintf("[%s: maxRPM=%d -> endRPM=%d] ", pwm.Name, maxRPM, lastRPM)
		time.Sleep(1 * time.Second)
	}

	ch <- autoDetectResultMsg{
		err: fmt.Errorf("no unassigned PWM channels caused RPM drop. Details: %s", debugLogs),
	}
}

func (m Model) saveRule() {
	if m.matchedPWM == nil {
		return
	}

	var selectedSensorIDs []string
	for idx, selected := range m.selectedTempIdxs {
		if selected && idx < len(m.telemetry.Temps) {
			sensorID := m.telemetry.Temps[idx].ID
			if sensorID != "" {
				selectedSensorIDs = append(selectedSensorIDs, sensorID)
			}
		}
	}

	if len(selectedSensorIDs) == 0 && len(m.telemetry.Temps) > 0 {
		fallbackID := m.telemetry.Temps[0].ID
		if fallbackID != "" {
			selectedSensorIDs = append(selectedSensorIDs, fallbackID)
		}
	}

	defaultCurve := []config.CurvePoint{
		{Temp: 35, PWM: m.calibMinPWM},
		{Temp: 75, PWM: m.calibMaxPWM},
	}

	fanName := ""
	if m.selectedFanIdx < len(m.telemetry.Fans) {
		fanName = m.telemetry.Fans[m.selectedFanIdx].Name
	}

	found := false
	for i, rule := range m.cfg.Rules {
		if rule.PWMID == m.matchedPWM.ID {
			m.cfg.Rules[i].FanName = fanName
			m.cfg.Rules[i].SensorIDs = selectedSensorIDs
			m.cfg.Rules[i].SensorAggMode = config.AggregationMode(m.calibAggMode)
			m.cfg.Rules[i].MinPWM = m.calibMinPWM
			m.cfg.Rules[i].MaxPWM = m.calibMaxPWM
			if len(m.cfg.Rules[i].Curve) == 0 {
				m.cfg.Rules[i].Curve = defaultCurve
			}
			found = true
			break
		}
	}

	if !found {
		m.cfg.Rules = append(m.cfg.Rules, config.FanRule{
			FanName:       fanName,
			PWMID:         m.matchedPWM.ID,
			Mode:          "auto",
			SensorIDs:     selectedSensorIDs,
			SensorAggMode: config.AggregationMode(m.calibAggMode),
			MinPWM:        m.calibMinPWM,
			MaxPWM:        m.calibMaxPWM,
			Curve:         defaultCurve,
		})
	}

	_ = saveRemoteConfig(m.apiPort, m.cfg)
}

func (m Model) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1).
		Render("=== FANCTL - Calibration Wizard ===")

	var body string

	switch m.currentStep {
	case StepSelectFan:
		body = "1. Select Fan Sensor to Calibrate:\n\n"

		tempMap := make(map[string]hwmon.TempSensor)
		for _, ts := range m.telemetry.Temps {
			tempMap[ts.ID] = ts
		}

		for i, fan := range m.telemetry.Fans {
			cursor := " "
			if i == m.selectedFanIdx {
				cursor = ">"
			}

			rule := m.findRuleForFan(i)
			badge := ""
			sensorInfo := ""

			if rule != nil {
				badge = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(fmt.Sprintf(" [Configured: PWM %s]", rule.PWMID))
				
				var sensorDetails []string
				for _, sid := range rule.SensorIDs {
					if ts, ok := tempMap[sid]; ok {
						sensorDetails = append(sensorDetails, fmt.Sprintf("%s: %.1f°C", ts.Name, ts.Value))
					}
				}
				if len(sensorDetails) > 0 {
					sensorInfo = fmt.Sprintf(" (Sensors: %s)", fmt.Sprint(sensorDetails))
				} else {
					sensorInfo = " (Sensors: none)"
				}
			} else {
				badge = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" [Not Configured]")
			}

			body += fmt.Sprintf(" %s %s (%d RPM)%s%s\n", cursor, fan.Name, fan.RPM, badge, sensorInfo)
		}
		body += "\n[Enter] Select/Configure  [q] Quit"

	case StepConfirmReset:
		body = "⚠️ This fan is already configured!\n\n"
		body += "Do you want to reset its configuration and reconfigure it from scratch?\n\n"

		yesMarker := " "
		noMarker := " "
		if m.confirmResetYes {
			yesMarker = ">"
		} else {
			noMarker = ">"
		}

		body += fmt.Sprintf(" %s Yes, reset and reconfigure\n", yesMarker)
		body += fmt.Sprintf(" %s No, go back\n\n", noMarker)
		body += "[↑/↓/←/→] Toggle  [Enter] Confirm  [Esc] Back"

	case StepAutoDetect:
		body = "2. Auto-Detecting PWM Channel...\n\n"
		if m.isDetecting {
			body += fmt.Sprintf(" Status: %s\n", m.detectStatus)
		} else if m.detectErr != nil {
			body += fmt.Sprintf(" Error: %v\n\n[Esc] Go Back", m.detectErr)
		}

	case StepAdjustLimits:
		currentRPM := 0
		if m.selectedFanIdx < len(m.telemetry.Fans) {
			currentRPM = m.telemetry.Fans[m.selectedFanIdx].RPM
		}

		matchedName := "Unknown"
		if m.matchedPWM != nil {
			matchedName = m.matchedPWM.Name
		}

		body = fmt.Sprintf("3. Adjust Speed Limits (Live Test):\nMatched PWM: %s\n\n", matchedName)
		body += fmt.Sprintf(" Current Speed: %d RPM\n\n", currentRPM)

		minCursor := " "
		maxCursor := " "
		if m.calibFocusMin == 1 {
			minCursor = ">"
		} else {
			maxCursor = ">"
		}

		body += fmt.Sprintf(" %s Minimum PWM: %d / 255\n", minCursor, m.calibMinPWM)
		body += fmt.Sprintf(" %s Maximum PWM: %d / 255\n\n", maxCursor, m.calibMaxPWM)
		body += "[↑/↓] Switch Min/Max  [←/→] Adjust Value  [Enter] Save Limits  [Esc] Back"

	case StepSelectSensors:
		body = "4. Select Target Temperature Sensor(s):\n\n"
		for i, temp := range m.telemetry.Temps {
			cursor := " "
			if i == m.selectedTempIdx {
				cursor = ">"
			}
			checked := "[ ]"
			if m.selectedTempIdxs[i] {
				checked = "[x]"
			}
			body += fmt.Sprintf(" %s %s %s (%.1f °C)\n", cursor, checked, temp.Name, temp.Value)
		}
		body += "\n[Space] Toggle  [Enter] Save Rule  [Esc] Back"

	case StepSummary:
		matchedName := "Unknown"
		if m.matchedPWM != nil {
			matchedName = m.matchedPWM.Name
		}
		body = "5. Calibration Saved Successfully via API!\n\n"
		body += fmt.Sprintf("Rule created for %s.\nConfig synchronized with daemon.\n\n", matchedName)
		body += "[Enter] Return to start  [q] Quit"
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}
