package grabber

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestXPubAddressGenerator_GenerateAddressRange(t *testing.T) {
	xpub := "vpub5VwWv7DDqNmuvVHB8KSXiTJ6w7nSwnp9YZZ7tMpr9Aon4DTTFPx4nAKRQPyo3JJ2qLhB9ekPKUrgWVGQ58QUybuFHYDQy3coXmDtaxePJxL"
	gen, err := NewXPubAddressGenerator(xpub, []uint32{0}, &chaincfg.TestNet3Params)
	require.NoError(t, err)
	results, err := gen.AddressRange(0, 50_000)
	require.NoError(t, err)
	t.Log(len(results))
}
