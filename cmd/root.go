// Package cmd is our cobra/viper cli implementation
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	appName        = "analyzer"
	appDescription = "analyzer of github prs"
)

var (
	logger   *zap.Logger
	cfgFile  string
	logLevel string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   appName,
	Short: appDescription,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		if logger != nil {
			logger.Debug("starting up...")
		}
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		if logger != nil {
			logger.Debug("shutting down...")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). This only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
}

// initConfig reads in config file and initializes the logger
func initConfig() {
	// Set up viper first to read config file
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
	}

	viper.AutomaticEnv()

	// Read config file if it exists (before initializing logger)
	_ = viper.ReadInConfig() // Ignore error, will use defaults if file doesn't exist

	// Initialize logger with level from config file, flag, or environment
	logger = configureLogger()
}

func mustSync() {
	if logger != nil {
		if err := logger.Sync(); err != nil {
			// Ignore sync errors on stdout/stderr
			_ = err
		}
	}
}

func configureLogger() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()

	// Set log level from (in order of precedence):
	// 1. CLI flag (--log-level)
	// 2. Config file (logging.level)
	// 3. Environment variable (LOG_LEVEL)
	// 4. Default (info)
	level := logLevel
	if level == "" {
		// Check config file
		level = viper.GetString("logging.level")
	}
	if level == "" {
		// Check environment variable
		level = os.Getenv("LOG_LEVEL")
	}
	if level == "" {
		level = "info"
	}

	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	return logger
}
