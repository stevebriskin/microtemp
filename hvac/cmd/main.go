package main

import (
	"context"
	_ "embed"

	"github.com/stevebriskin/microtemp/hvac"

	"github.com/edaniels/golog"
)

//go:embed config.json
var CONFIG_BYTES []byte

func main() {
	logger := golog.NewDebugLogger("client")
	ctx := context.Background()

	hvac.ParseConfig(CONFIG_BYTES, &hvac.Conf)
	logger.Infof("Config: %+v", hvac.Conf)

	hvac.DoStuff(ctx, logger)
}
