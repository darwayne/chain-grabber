package memspender

import (
	"context"
	"database/sql"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/noderpc"
	"github.com/darwayne/chain-grabber/pkg/keygen"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/darwayne/chain-grabber/pkg/txmonitor"
	"github.com/stretchr/testify/require"
	"log"
	"os"
	"testing"
	"time"
)

func TestMemPoolSub(t *testing.T) {
	mon := txmonitor.NewZeroMonitor("10.0.0.20:38333")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		close(done)
	})

	go func() {
		channel := mon.Subscribe()

		for {
			select {
			case tx := <-channel:
				log.Printf("got tx [%s]\n", tx.TxHash())
			case <-done:
				return
			}
		}

	}()

	go func() {
		sigutil.Wait()
		cancel()
		t.Fatal("ending")
	}()
	mon.Start(ctx)
}

func TestChainAnalyzer(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")
	ctx := context.Background()

	cli, err := noderpc.NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)

	endBlock := 843464

	result, err := cli.GetBlockFromHeight(ctx, endBlock)
	require.NoError(t, err)
	t.Log(result.BlockHash())

	a := NewChainAnalyzer(_analyzerSecretStore(t), cli)

	const daysBack = 365

	startBlock := endBlock - (daysBack * 144)

	//endBlock = 843_848
	//startBlock = endBlock

	seen, err := a.AnalyzeAll(ctx, startBlock, endBlock)
	require.NoError(t, err)

	t.Log("known keys seen:", seen)

}

func _analyzerSecretStore(t testing.TB) secretStore {
	t.Helper()
	db, err := sql.Open("sqlite3", "../keygen/testdata/sampledbs/keydbv2.sqlite")
	require.NoError(t, err)
	reader1 := keygen.NewSQLReader(db, &chaincfg.MainNetParams)
	t.Cleanup(func() {
		reader1.Close()
	})

	return reader1

	//db2, err := sql.Open("sqlite3", "../keygen/testdata/sampledbs/keys_only.sqlite")
	//require.NoError(t, err)
	//reader2 := keygen.NewSQLReader(db2, &chaincfg.MainNetParams)
	//t.Cleanup(func() {
	//	reader2.Close()
	//})
	//
	//db3, err := sql.Open("sqlite3", "../keygen/testdata/sampledbs/keys_only.sqlite")
	//require.NoError(t, err)
	//reader3 := keygen.NewSQLReader(db3, &chaincfg.MainNetParams)
	//t.Cleanup(func() {
	//	reader3.Close()
	//})

	//return keygen.NewMultiReader(reader1, reader2, reader3)
}
