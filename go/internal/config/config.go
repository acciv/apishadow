package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Listen      string
	Primary     string
	Shadow      string
	CaptureFile string
	MirrorRatio float64
	TimeoutMS   int
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	cfg := &Config{
		Listen:      ":8080",
		Primary:     "http://127.0.0.1:9101",
		Shadow:      "http://127.0.0.1:9102",
		CaptureFile: "capture.jsonl",
		MirrorRatio: 1.0,
		TimeoutMS:   3000,
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "listen":
			cfg.Listen = value
		case "primary":
			cfg.Primary = value
		case "shadow":
			cfg.Shadow = value
		case "capture_file":
			cfg.CaptureFile = value
		case "mirror_ratio":
			if n, err := strconv.ParseFloat(value, 64); err == nil {
				cfg.MirrorRatio = n
			}
		case "timeout_ms":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.TimeoutMS = n
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if cfg.Primary == "" || cfg.Shadow == "" {
		return nil, fmt.Errorf("primary and shadow are required")
	}
	return cfg, nil
}
