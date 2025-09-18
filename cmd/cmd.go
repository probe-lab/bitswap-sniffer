package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/probe-lab/bitswap-sniffer/bitswap"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var rootConfig = struct {
	LogLevel            string
	LogFormat           string
	MetricsHost         string
	MetricsPort         int
	Logger              *logrus.Logger
	MetricsProvider     metric.MeterProvider
	MetricsShutdownFunc func(context.Context) error
}{
	LogLevel:            "info",
	LogFormat:           "text",
	MetricsHost:         "127.0.0.1",
	MetricsPort:         9080,
	Logger:              nil,
	MetricsShutdownFunc: nil,
}

var rootCmd = &cli.Command{
	Name:                  "bitswap-sniffer",
	Usage:                 "",
	EnableShellCompletion: true,
	Flags:                 rootFlags,
	Before:                rootBefore,
	After:                 rootAfter,
	Commands: []*cli.Command{
		cmdRun,
	},
}

var rootFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "log.level",
		Usage:       "Level of the logs",
		Value:       rootConfig.LogLevel,
		Destination: &rootConfig.LogLevel,
	},
	&cli.StringFlag{
		Name:        "log.format",
		Usage:       "Format of the logs [text, json]",
		Value:       rootConfig.LogLevel,
		Destination: &rootConfig.LogLevel,
	},
	&cli.StringFlag{
		Name:        "metrics.host",
		Usage:       "IP for the metrics OP host",
		Value:       rootConfig.MetricsHost,
		Destination: &rootConfig.MetricsHost,
	},
	&cli.IntFlag{
		Name:        "metrics.port",
		Usage:       "Port for the metrics OP host",
		Value:       rootConfig.MetricsPort,
		Destination: &rootConfig.MetricsPort,
	},
}

func main() {
	// Set log level from environment variable
	if level := os.Getenv("LOGRUS_LEVEL"); level != "" {
		if parsedLevel, err := logrus.ParseLevel(level); err == nil {
			logrus.SetLevel(parsedLevel)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rootCmd.Run(ctx, os.Args); err != nil && !errors.Is(err, context.Canceled) {
		logrus.Error(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func rootBefore(c context.Context, cmd *cli.Command) (context.Context, error) {
	if cmd.NArg() == 0 {
		return c, nil
	}

	// read CLI args and configure the global logger
	rootConfig.Logger = logrus.New()
	if err := configureLogger(c, cmd, rootConfig.Logger); err != nil {
		return c, err
	}

	// read CLI args and configure the global meter provider
	if err := configureMetrics(c, cmd); err != nil {
		return c, err
	}

	logrus.WithFields(logrus.Fields{
		"log-level":    rootConfig.LogLevel,
		"log-format":   rootConfig.LogFormat,
		"metrics-host": rootConfig.MetricsHost,
		"metrics-port": rootConfig.MetricsPort,
	}).Info("running bitswap-sniffer with ...")
	return c, nil
}

func rootAfter(c context.Context, cmd *cli.Command) error {
	logrus.Info("successfully shutted down")
	return nil
}

func configureLogger(_ context.Context, cmd *cli.Command, logger *logrus.Logger) error {
	// log level
	logLevel := logrus.InfoLevel
	if cmd.IsSet("log.level") {
		switch strings.ToLower(rootConfig.LogLevel) {
		case "debug":
			logLevel = logrus.DebugLevel
		case "info":
			logLevel = logrus.InfoLevel
		case "warn":
			logLevel = logrus.WarnLevel
		case "error":
			logLevel = logrus.ErrorLevel
		default:
			return fmt.Errorf("unknown log level: %s", rootConfig.LogLevel)
		}
	}
	logger.SetLevel(logrus.Level(logLevel))

	// log format
	switch strings.ToLower(rootConfig.LogFormat) {
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			DisableColors: false,
		})
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		return fmt.Errorf("unknown log format: %q", rootConfig.LogFormat)
	}

	return nil
}

func configureMetrics(ctx context.Context, _ *cli.Command) error {
	// user wants to have metrics, use the prometheus meter provider
	provider, err := bitswap.PromMeterProvider(ctx)
	if err != nil {
		return fmt.Errorf("new prometheus meter provider: %w", err)
	}

	otel.SetMeterProvider(provider)

	// expose the /metrics endpoint. Use new context, so that the metrics server
	// won't stop when an interrupt is received. If the shutdown procedure hangs
	// this will give us a chance to still query pprof or the metrics endpoints.
	shutdownFunc := bitswap.ServeMetrics(ctx, rootConfig.MetricsHost, rootConfig.MetricsPort)

	rootConfig.MetricsShutdownFunc = shutdownFunc
	rootConfig.MetricsProvider = provider

	return nil
}
