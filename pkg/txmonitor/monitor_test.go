package txmonitor

import (
	"context"
	"fmt"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"sync/atomic"
	"testing"
	"time"
)

func TestMainNet(t *testing.T) {
	testNetwork(t, true)
}

func TestTestNet(t *testing.T) {
	testNetwork(t, false)
}

func testNetwork(t *testing.T, isMainNet bool) {
	m := New()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	l, err := zap.NewDevelopment()
	require.NoError(t, err)
	var n *lightnode.Node
	if isMainNet {
		n, err = lightnode.NewMainNet(l)
	} else {
		n, err = lightnode.NewTestNet(l)
	}

	require.NoError(t, err)
	go n.ConnectV2()
	info := m.Subscribe()

	go m.Start(ctx, n)
	var seen int64

	start := time.Now()

	go func() {
		for {
			select {
			case tx := <-info:
				go func() {
					val := atomic.AddInt64(&seen, 1)
					duration := time.Since(start)
					fmt.Println(val, "saw tx", tx.TxHash(), "msg/s:",
						fmt.Sprintf("%.2f", float64(val)/duration.Seconds()),
						"elapsed:", duration)
				}()

			}
		}
	}()

	t.Cleanup(func() {
		require.NotZero(t, atomic.LoadInt64(&seen))
	})

	select {
	case <-ctx.Done():
		return
	case <-n.Done():
		return
	case <-sigutil.Done():
		return
	}
}
