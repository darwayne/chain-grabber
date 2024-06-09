package grabber

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRange_Split(t *testing.T) {
	r := Range{0, 50_000}

	rr := r.Split(12)
	t.Logf("length: (%d)\n%+v", len(rr), rr)
}

func BenchmarkGenerateKeys(b *testing.B) {
	start := time.Now()
	generated, err := GenerateKeys(0, 500_000, &chaincfg.TestNet3Params)
	require.NoError(b, err)
	_ = generated
	b.Log(time.Since(start))
	//for i := 0; i < b.N; i++ {
	//	GenerateKeys(0, 100, &chaincfg.TestNet3Params)
	//}
}

func TestStreamKeysOnlyOrdered(t *testing.T) {
	for info := range StreamKeysOnlyOrdered(1, 1000) {
		t.Log(info.Num)
	}

	sigutil.Wait()
}
