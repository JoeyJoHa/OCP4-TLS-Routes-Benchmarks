package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/blobs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/certs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/server"
)

const urandomDevice = "/dev/urandom"

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if err := run(); err != nil {
		log.Fatalf("tlsbench: %v", err)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	random, err := os.Open(urandomDevice)
	if err != nil {
		return fmt.Errorf("open %s: %w", urandomDevice, err)
	}
	defer random.Close()

	store, err := blobs.NewStore(cfg.DataDir, cfg.MaxBlobBytes, random)
	if err != nil {
		return fmt.Errorf("blob store: %w", err)
	}
	logger, err := results.NewLogger(cfg.ResultsLog)
	if err != nil {
		return fmt.Errorf("results log: %w", err)
	}
	material, err := certs.LoadOrGenerate(cfg)
	if err != nil {
		return fmt.Errorf("tls material: %w", err)
	}
	if material.Generated {
		log.Printf("generated self-signed CA and server certificate (SANs: %v)", cfg.TLSDNSNames)
	} else {
		log.Printf("loaded server certificate from %s", cfg.TLSCertFile)
	}

	if _, err := certs.LoadExtraCAs(cfg.TLSCADir); err != nil {
		return fmt.Errorf("load extra CAs from %s: %w", cfg.TLSCADir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := server.New(cfg, store, logger, material)
	return app.ListenAndServe(ctx)
}
