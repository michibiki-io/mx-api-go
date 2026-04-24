package logging

import (
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(mode string) (*zap.Logger, error) {
	if strings.EqualFold(mode, "debug") {
		return zap.NewDevelopment()
	}

	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		jst := time.FixedZone("Asia/Tokyo", 9*60*60)
		enc.AppendString(t.In(jst).Format("2006-01-02 15:04:05 JST"))
	}
	return cfg.Build()
}
