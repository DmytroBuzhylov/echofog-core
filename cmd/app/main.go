package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DmytroBuzhylov/echofog-core/internal/config"
	"github.com/DmytroBuzhylov/echofog-core/pkg/node"
)

func main() {
	cfg := config.DefaultConfig()
	cfg.Network.ListenAddr = "0.0.0.0:4242"
	cfg.Storage.DatabasePath = "./data"

	echoNode := node.NewNode(cfg)

	go func() {
		for logEntry := range echoNode.GetLogChannel() {
			fmt.Printf("[%s] %s: %s\n", logEntry.Time, logEntry.Level, logEntry.Message)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Enter password to unlock identity:")
	var password string
	fmt.Scanln(&password)

	if err := echoNode.Start(ctx, password); err != nil {
		fmt.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down EchoFog...")
	echoNode.Stop()
}
