package keygen

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBrainKeys(t *testing.T) {
	cfg := &chaincfg.TestNet3Params
	key, err := BrainKey("hello", cfg)
	require.NoError(t, err)

	yo, err := key.Derive(9000)
	require.NoError(t, err)

	for i := 0; i < 300; i++ {
		kk, err := yo.Derive(uint32(i))
		require.NoError(t, err)
		priv, err := kk.ECPrivKey()
		require.NoError(t, err)
		addr, err := grabber.PrivToTaprootPubKey(priv, cfg)
		require.NoError(t, err)
		t.Logf("%d -  %s; depth: %d", i, addr, kk.Depth())

		neutered, err := key.Neuter()
		require.NoError(t, err)
		t.Log("key", kk.String(), "neutered:", neutered.String())
	}
}
