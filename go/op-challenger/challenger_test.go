package op_challenger

import (
	"context"
	"testing"

	"github.com/ethereum-optimism/optimism/op-service/testlog"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/go/op-challenger/config"
	"github.com/ethereum-optimism/optimism/go/op-challenger/metrics"
)

func TestMainShouldReturnErrorWhenConfigInvalid(t *testing.T) {
	cfg := &config.Config{}
	app, err := Main(context.Background(), testlog.Logger(t, log.LevelInfo), cfg, metrics.NoopMetrics)
	require.ErrorIs(t, err, cfg.Check())
	require.Nil(t, app)
}