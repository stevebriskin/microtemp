package readtemp

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/stevebriskin/microtemp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/edaniels/golog"
)

/*
   This is an example of what a Google Cloud Function may look like.
*/

//go:embed testconfig.json
var CONFIG_BYTES []byte
var conf microtemp.Config
var logger *zap.SugaredLogger

func docloudstuff() {
	microtemp.DoAll(context.Background(), logger)
}

func parseConfig(conf *microtemp.Config) error {
	if err := json.Unmarshal(CONFIG_BYTES, conf); err != nil {
		return err
	}

	return nil
}

func makeLogger(name string) golog.Logger {
	cfg := zap.Config{
		Level:    zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding: "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		DisableStacktrace: true,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}

	logger, _ := cfg.Build()
	return logger.Sugar().Named(name)
}

func init() {
	logger = makeLogger("client")

	err := parseConfig(&conf)
	if err != nil {
		logger.Fatal(err)
	}

	err = microtemp.Initialize(context.Background(), conf, logger)
	if err != nil {
		logger.Fatal(err)
	}

	functions.HTTP("ReadTemp", readtempHttp)
}

func readtempHttp(w http.ResponseWriter, r *http.Request) {
	docloudstuff()
}
