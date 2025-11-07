package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// var (
// 	// Static is the embedded filesystems for static files
// 	Static embed.FS
// 	// Templates is the embedded filesystems for templates
// 	Templates embed.FS
// )

// serveCmd starts the API server
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: fmt.Sprintf("starts the %s server", appName),
	Run: func(c *cobra.Command, _ []string) {
		defer mustSync()
		startAPI(c.Context())
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func startAPI(cmdCtx context.Context) {
	sugar := logger.Sugar()
	sugar.Infof("Starting %s serve... ", appName)

	// use this ctx when starting the server
	_, cancel := context.WithCancel(cmdCtx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// db := initDB()
	// dbtools.RegisterHooks()
	// Run the embedded migration in the event that this is the first
	// run or first run since a new migration was added.
	// RunMigration(db.DB)

	// opts := []server.Option{
	// 	server.WithListener(cfg.Listen),
	// 	server.WithLogger(logger),
	// }

	// s, err := server.NewServer(opts...)
	// if err != nil {
	// 	logger.Fatal("failed to create server", zap.Error(err))
	// }

	// if err := s.Start(ctx); err != nil {
	// 	logger.Fatal("failed starting server", zap.Error(err))
	// }

	<-c
	cancel()
	sugar.Infof("Shutting down the %s server...", appName)
}
