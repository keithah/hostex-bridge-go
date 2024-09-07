package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
	"go.mau.fi/util/configupgrade"
	"go.uber.org/zap"

	"github.com/keithah/hostex-bridge-go/config"
	"github.com/keithah/hostex-bridge-go/database"
	"github.com/keithah/hostex-bridge-go/hostexapi"
)

var (
	configPath = flag.String("config", "config.yaml", "Path to config file")
	verbose    = flag.Bool("v", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	// Initialize logging
	logConfig := zap.NewDevelopmentConfig()
	if *verbose {
		logConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	logger, err := logConfig.Build()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Initialize database
	db, err := database.New(cfg.Database.Path, logger)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize Hostex API client
	hostexClient := hostexapi.NewClient(cfg.Hostex.APIURL, cfg.Hostex.Token, logger)

	// Initialize Matrix client
	matrixClient, err := mautrix.NewClient(cfg.Homeserver.Address, "", "")
	if err != nil {
		logger.Fatal("Failed to create Matrix client", zap.Error(err))
	}

	// Initialize bridge
	bridge := NewBridge(cfg, db, hostexClient, matrixClient, logger)

	// Start the bridge
	err = bridge.Start()
	if err != nil {
		logger.Fatal("Failed to start bridge", zap.Error(err))
	}

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	// Stop the bridge
	bridge.Stop()
}
