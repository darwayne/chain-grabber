package keygen

import (
	"database/sql"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"runtime"
	"sort"
	"testing"
)

func TestNewSQLReader(t *testing.T) {
	db, err := sql.Open("sqlite3", "./testdata/sampledbs/keydbv2.sqlite")
	require.NoError(t, err)
	params := &chaincfg.TestNet3Params
	secretStore := NewSQLReader(db, params)

	l, err := zap.NewDevelopment()

	fmt.Println(runtime.GOMAXPROCS(0))

	l.Info("generating keys")
	start := 1
	incr := start + 200
	info, err := grabber.GenerateKeys(start, incr, params)
	require.NoError(t, err)
	l.Info("key generation complete!",
		zap.Int("total", len(info.AddressKeyMap)))

	var addrs []string
	for k, v := range info.AddressKeyMap {
		_ = v
		addrs = append(addrs, k)
	}

	sort.Slice(addrs, func(i, j int) bool {
		return info.AddressOrder[addrs[i]] < info.AddressOrder[addrs[j]]
	})

	for _, k := range addrs {
		addr, err := btcutil.DecodeAddress(k, params)
		require.NoError(t, err)
		v, found := info.AddressKeyMap[k]
		require.True(t, found)
		hasAddress, err := secretStore.HasAddress(addr)
		require.NoError(t, err)
		require.True(t, hasAddress)
		hasKey, err := secretStore.HasKey(v.SerializePubKey())
		require.NoError(t, err)
		require.True(t, hasKey)

		//t.Log("checking address", addr, "compresses:", v.CompressPubKey)
		//t.Log("=====")
		//t.Log("address check:")
		//t.Log(secretStore.HasAddress(addr))
		//t.Log(info.AddressOrder[k])
		//
		////if v.CompressPubKey {
		//t.Log("key check:", len(v.SerializePubKey()))
		//t.Log(secretStore.HasKey(v.SerializePubKey()))
		////}
		//
		//t.Log("=-=-=-=-")
	}

}
