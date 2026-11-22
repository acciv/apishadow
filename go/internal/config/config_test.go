package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apishadow.conf")
	content := "# comment\nlisten=:9090\nprimary=http://a\nshadow=http://b\nmirror_ratio=0.5\ntimeout_ms=120\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9090" || cfg.Primary != "http://a" || cfg.Shadow != "http://b" || cfg.MirrorRatio != 0.5 || cfg.TimeoutMS != 120 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
