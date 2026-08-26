package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type jsonDuration time.Duration

func (d *jsonDuration) UnmarshalJSON(data []byte) error {
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("empty duration")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		parsed, err := time.ParseDuration(text)
		if err != nil {
			return err
		}
		*d = jsonDuration(parsed)
		return nil
	}
	var nanos float64
	if err := json.Unmarshal(raw, &nanos); err != nil {
		return err
	}
	*d = jsonDuration(time.Duration(nanos))
	return nil
}

type Config struct {
	ListenAddr       string        `json:"listen_addr"`
	DataDir          string        `json:"data_dir"`
	ShoreCapacity    float64       `json:"shore_capacity_kva"`
	VoltageKV        float64       `json:"voltage_kv"`
	FreqHz           float64       `json:"freq_hz"`
	VerifyTimeout    time.Duration `json:"verify_timeout"`
	SyncTimeout      time.Duration `json:"sync_timeout"`
	SampleEvery      time.Duration `json:"sample_every"`
	ReconcileEvery   time.Duration `json:"reconcile_every"`
	PeakStartHour    int           `json:"peak_start_hour"`
	PeakEndHour      int           `json:"peak_end_hour"`
	ValleyStartHour  int           `json:"valley_start_hour"`
	ValleyEndHour    int           `json:"valley_end_hour"`
	EnergyUnitPrice  float64       `json:"energy_unit_price"`
	ServiceUnitPrice float64       `json:"service_unit_price"`
}

func Default() Config {
	return Config{
		ListenAddr:       "127.0.0.1:8095",
		DataDir:          "./data",
		ShoreCapacity:    6000,
		VoltageKV:        10,
		FreqHz:           50,
		VerifyTimeout:    10 * time.Second,
		SyncTimeout:      3 * time.Second,
		SampleEvery:      time.Second,
		ReconcileEvery:   30 * time.Second,
		PeakStartHour:    8,
		PeakEndHour:      22,
		ValleyStartHour:  22,
		ValleyEndHour:    8,
		EnergyUnitPrice:  0.85,
		ServiceUnitPrice: 0.15,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	var file struct {
		ListenAddr       string       `json:"listen_addr"`
		DataDir          string       `json:"data_dir"`
		ShoreCapacity    float64      `json:"shore_capacity_kva"`
		VoltageKV        float64      `json:"voltage_kv"`
		FreqHz           float64      `json:"freq_hz"`
		VerifyTimeout    jsonDuration `json:"verify_timeout"`
		SyncTimeout      jsonDuration `json:"sync_timeout"`
		SampleEvery      jsonDuration `json:"sample_every"`
		ReconcileEvery   jsonDuration `json:"reconcile_every"`
		PeakStartHour    int          `json:"peak_start_hour"`
		PeakEndHour      int          `json:"peak_end_hour"`
		ValleyStartHour  int          `json:"valley_start_hour"`
		ValleyEndHour    int          `json:"valley_end_hour"`
		EnergyUnitPrice  float64      `json:"energy_unit_price"`
		ServiceUnitPrice float64      `json:"service_unit_price"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if file.ListenAddr != "" {
		cfg.ListenAddr = file.ListenAddr
	}
	if file.DataDir != "" {
		cfg.DataDir = file.DataDir
	}
	if file.ShoreCapacity != 0 {
		cfg.ShoreCapacity = file.ShoreCapacity
	}
	if file.VoltageKV != 0 {
		cfg.VoltageKV = file.VoltageKV
	}
	if file.FreqHz != 0 {
		cfg.FreqHz = file.FreqHz
	}
	if file.VerifyTimeout != 0 {
		cfg.VerifyTimeout = time.Duration(file.VerifyTimeout)
	}
	if file.SyncTimeout != 0 {
		cfg.SyncTimeout = time.Duration(file.SyncTimeout)
	}
	if file.SampleEvery != 0 {
		cfg.SampleEvery = time.Duration(file.SampleEvery)
	}
	if file.ReconcileEvery != 0 {
		cfg.ReconcileEvery = time.Duration(file.ReconcileEvery)
	}
	if file.PeakStartHour != 0 {
		cfg.PeakStartHour = file.PeakStartHour
	}
	if file.PeakEndHour != 0 {
		cfg.PeakEndHour = file.PeakEndHour
	}
	if file.ValleyStartHour != 0 {
		cfg.ValleyStartHour = file.ValleyStartHour
	}
	if file.ValleyEndHour != 0 {
		cfg.ValleyEndHour = file.ValleyEndHour
	}
	if file.EnergyUnitPrice != 0 {
		cfg.EnergyUnitPrice = file.EnergyUnitPrice
	}
	if file.ServiceUnitPrice != 0 {
		cfg.ServiceUnitPrice = file.ServiceUnitPrice
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = Default().ListenAddr
	}
	if cfg.DataDir == "" {
		cfg.DataDir = Default().DataDir
	}
	if cfg.ShoreCapacity <= 0 {
		return cfg, fmt.Errorf("shore capacity must be positive")
	}
	if cfg.VerifyTimeout <= 0 || cfg.SyncTimeout <= 0 {
		return cfg, fmt.Errorf("timeouts must be positive")
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen addr is required")
	}
	if c.ShoreCapacity <= 0 {
		return fmt.Errorf("shore capacity must be positive")
	}
	if c.VoltageKV <= 0 || c.FreqHz <= 0 {
		return fmt.Errorf("voltage and frequency must be positive")
	}
	if c.VerifyTimeout <= 0 || c.SyncTimeout <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}
	if c.PeakStartHour < 0 || c.PeakStartHour > 23 || c.PeakEndHour < 0 || c.PeakEndHour > 23 {
		return fmt.Errorf("peak hours out of range")
	}
	return nil
}
