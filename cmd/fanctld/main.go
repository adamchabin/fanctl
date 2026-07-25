package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"fanctl/internal/api"
	"fanctl/internal/config"
	"fanctl/internal/engine"
	"fanctl/internal/hwmon"
)

func main() {
	log.SetFlags(0)
	log.Println("Starting fanctld daemon...")

	configPath := "/etc/fanctl/fanctl.conf"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Printf("No config file found or load error (%v), creating default %s...", err, configPath)
		cfg = config.DefaultConfig()
		if saveErr := cfg.Save(configPath); saveErr != nil {
			log.Fatalf("Failed to create default config file: %v", saveErr)
		}
	}

	var cfgLock sync.RWMutex
	scanner := hwmon.NewScanner()

	server := api.NewServer(configPath, cfg, &cfgLock, scanner)
	go func() {
		log.Printf("Starting API server on port %d...", cfg.APIPort)
		if err := server.Start(cfg.APIPort); err != nil {
			log.Fatalf("API Server error: %v", err)
		}
	}()

	stopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.PollIntervalSec) * time.Second)
		defer ticker.Stop()

		lastPWMValues := make(map[string]int)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				cfgLock.RLock()
				currentCfg := *cfg
				cfgLock.RUnlock()

				telemetry, err := scanner.GetTelemetry()
				if err != nil {
					log.Printf("Error fetching telemetry in control loop: %v", err)
					continue
				}

				tempMap := make(map[string]float64)
				for _, ts := range telemetry.Temps {
					tempMap[ts.ID] = ts.Value
				}

				pwmMap := make(map[string]hwmon.PWMControl)
				for _, p := range telemetry.PWMs {
					pwmMap[p.ID] = p
				}

				for _, rule := range currentCfg.Rules {
					if rule.Mode == "manual" {
						if pwmCtrl, ok := pwmMap[rule.PWMID]; ok {
							_ = scanner.SetPWM(pwmCtrl, rule.ManualPWM)
							if currentCfg.Debug {
								log.Printf("[DEBUG] Fan %s (PWM ID: %s) set to manual PWM: %d", rule.FanName, rule.PWMID, rule.ManualPWM)
							}
						}
						continue
					}

					aggTemp, ok := engine.AggregateTemperatures(rule.SensorIDs, tempMap, rule.SensorAggMode)
					if !ok {
						if currentCfg.Debug {
							log.Printf("[DEBUG] Fan %s: Failed to aggregate temperatures for sensors %v", rule.FanName, rule.SensorIDs)
						}
						continue
					}

					lastPWM := lastPWMValues[rule.PWMID]
					targetPWM := engine.CalculatePWM(aggTemp, rule, lastPWM)

					if currentCfg.Debug {
						log.Printf("[DEBUG] Fan: %s (PWM ID: %s) | Aggregated Temp: %.2f°C | Target PWM: %d", rule.FanName, rule.PWMID, aggTemp, targetPWM)
					}

					if pwmCtrl, ok := pwmMap[rule.PWMID]; ok {
						_ = scanner.SetPWM(pwmCtrl, targetPWM)
						lastPWMValues[rule.PWMID] = targetPWM
					} else if currentCfg.Debug {
						log.Printf("[DEBUG] Warning: PWM channel ID %s not found in system", rule.PWMID)
					}
				}
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigChan {
		if sig == syscall.SIGHUP {
			log.Println("Received SIGHUP, reloading configuration...")
			newCfg, loadErr := config.LoadConfig(configPath)
			if loadErr != nil {
				log.Printf("Failed to reload configuration: %v", loadErr)
			} else {
				cfgLock.Lock()
				cfg = newCfg
				cfgLock.Unlock()
				log.Println("Configuration reloaded successfully.")
			}
			continue
		}
		break
	}

	log.Println("Shutting down fanctld daemon...")
	close(stopChan)
	time.Sleep(500 * time.Millisecond)
}
