// Package cmd is our cobra/viper cli implementation
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	appName        = "CHANGEME"
	appDescription = "CHANGEME"
)

var (
	configFile string

	cpuProfile     string
	cpuProfileFile *os.File

	logRuntimeStats bool

	logger *zap.Logger
	// cfg    *CHANGEME.Config

	// defaultLogLevel     = zapcore.InfoLevel
	// defaultForwardLevel = zapcore.InfoLevel
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   appName,
	Short: appDescription,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		if cpuProfile != "" {
			var err error
			cpuProfileFile, err = os.Create(cpuProfile)
			if err != nil {
				log.Fatal(err)
			}

			if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
				log.Fatal(err)
			}
		}

		if logRuntimeStats {
			logger.Info("dumpting runtime stats")
			go func(ctx context.Context) {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					var memStats runtime.MemStats
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						runtime.ReadMemStats(&memStats)
						logger.Info("runtime stats", zap.Any("mem_stats", memStats))
					}
				}
			}(cmd.Context())
		}
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		if cpuProfile != "" {
			pprof.StopCPUProfile()
			cpuProfileFile.Close()
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "config.yaml", "config file (default is config.yaml)")
	rootCmd.PersistentFlags().StringVar(&cpuProfile, "cpu_profile", "", "cpu profile file")
	rootCmd.PersistentFlags().BoolVar(&logRuntimeStats, "log_runtime_stats", false, "print runtime stats to the log")
}

// initConfig reads in config file and initializes the logger
func initConfig() {
	fmt.Println("initConfig")
	// // Load configuration
	// cfg = CHANGEME.NewDefaultConfig()
	// if err := CHANGEME.Load(configFile, configEnv); err != nil {
	// 	panic(err)
	// }

	// logger = configureLogger(context.Background(), cfg)

	// logger.Debug("config", zap.Any("cfg", cfg))
}

func mustSync() {
	if err := logger.Sync(); err != nil {
		// sync behavior is different depending on the platform, ignore EINVAL
		// reference: https://github.com/uber-go/zap/issues/328
		if errors.Is(err, syscall.EINVAL) {
			return
		}

		panic(err)
	}
}

// func configureLogger(ctx context.Context, cfg *crp.Config) *zap.Logger {
// 	var logger *zap.Logger

// 	logLevel := defaultLogLevel

// 	if cfg.LogLevel != "" {
// 		var err error

// 		logLevel, err = zapcore.ParseLevel(cfg.LogLevel)
// 		if err != nil {
// 			log.Fatalf("failed to parse log level %s", err)
// 		}
// 	}

// 	logLeveler := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
// 		return level >= logLevel
// 	})

// 	stdoutSyncer := zapcore.Lock(os.Stdout)

// 	zapCores := []zapcore.Core{
// 		zapcore.NewCore(
// 			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
// 			stdoutSyncer,
// 			logLeveler,
// 		),
// 	}

// 	// if cfg.ForwardLogs != nil {
// 	// 	if cfg.ForwardLogs.Destinations.Type != "cloudwatchlogs" {
// 	// 		log.Fatalf("unsupported forward log destination type %s", cfg.ForwardLogs.Destinations.Type)
// 	// 	}

// 	// 	logGroup := cfg.ForwardLogs.Destinations.LogGroup
// 	// 	if logGroup == "" {
// 	// 		logGroup = fmt.Sprintf("%s-application-logs", appName)
// 	// 	}

// 	// 	log.Println("setting log group for cloudwatchlogs", logGroup)

// 	// 	logStream := cfg.ForwardLogs.Destinations.LogStream
// 	// 	if logStream == "" {
// 	// 		logStream = uuid.New().String()
// 	// 	}

// 	// 	log.Println("setting log stream for cloudwatchlogs", logStream)

// 	// 	envPrefix := "CWL"
// 	// 	if cfg.ForwardLogs.Destinations.EnvPrefix != "" {
// 	// 		envPrefix = cfg.ForwardLogs.Destinations.EnvPrefix
// 	// 	}

// 	// 	cwl := newCloudWatchLogs(
// 	// 		ctx,
// 	// 		newAWSCfg(ctx, cfg.ForwardLogs.Destinations.URL, NewEnvCredentialsProvider(envPrefix)),
// 	// 		logGroup,
// 	// 		logStream,
// 	// 	)

// 	// 	forwardLevel := defaultForwardLevel

// 	// 	if cfg.ForwardLogs.Level != "" {
// 	// 		var err error

// 	// 		forwardLevel, err = zapcore.ParseLevel(cfg.ForwardLogs.Level)
// 	// 		if err != nil {
// 	// 			log.Fatalf("failed to parse forwarding log level %s", err)
// 	// 		}
// 	// 	}

// 	// 	// return true if the level is greater than or equal to the forward level
// 	// 	fwdLeveler := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
// 	// 		return level >= forwardLevel
// 	// 	})

// 	// 	zapCores = append(zapCores, zapcore.NewCore(
// 	// 		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
// 	// 		cwl,
// 	// 		fwdLeveler,
// 	// 	))
// 	// }

// 	logger = zap.New(zapcore.NewTee(zapCores...))

// 	return logger
// }
