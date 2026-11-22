package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/acciv/apishadow/internal/capture"
	"github.com/acciv/apishadow/internal/config"
	"github.com/acciv/apishadow/internal/proxy"
)

func main() {
	configPath := flag.String("config", "../config/apishadow.conf", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	writer, err := capture.NewFileWriter(cfg.CaptureFile)
	if err != nil {
		log.Fatalf("capture: %v", err)
	}
	defer writer.Close()

	p := proxy.New(cfg.Primary, cfg.Shadow, writer, cfg.MirrorRatio, time.Duration(cfg.TimeoutMS)*time.Millisecond)
	log.Printf("apishadow listening on %s (capture engine %s)", cfg.Listen, capture.Version())
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           p.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
