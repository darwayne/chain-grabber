package mempoolspace

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestUTXOBuilder(t *testing.T) {
	r := NewRest()
	ctx := context.Background()
	fmt.Println("allocating memory")
	utxoSet := make(map[wire.OutPoint]int64, 900_000)
	fmt.Println("memory allocated")
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	for start := 0; start < 10; start++ {
		time.Sleep(500 * time.Millisecond)
		hash, err := r.GetBlockHashFromHeight(ctx, start)
		require.NoError(t, err, "error at block height %d", start)
		require.NotNil(t, hash)
		block, err := r.GetBlock(ctx, *hash)
		for _, tx := range block.Transactions {
			for _, input := range tx.TxIn {
				delete(utxoSet, input.PreviousOutPoint)
			}

			for idx, output := range tx.TxOut {
				utxoSet[wire.OutPoint{Hash: tx.TxHash(), Index: uint32(idx)}] = output.Value
			}
		}
	}

	var amount btcutil.Amount
	for _, value := range utxoSet {
		amount += btcutil.Amount(value)
	}

	t.Log("total BTC available", amount)
	t.Log("total UTXOs available", len(utxoSet))
	fmt.Printf("TotalAlloc: %d\nHeapAlloc: %d\nHeapInuse: %d\n\n\n", m.TotalAlloc, m.HeapAlloc, m.HeapInuse)
	select {}

}

func TestRest_GetBlock(t *testing.T) {
	r := NewRest()

	hash, err := chainhash.NewHashFromStr("0000000000000000000065bda8f8a88f2e1e00d9a6887a43d640e52a4c7660f2")
	require.NoError(t, err)
	block, err := r.GetBlock(context.Background(), *hash)
	for idx, tx := range block.Transactions {
		_ = tx
		if idx == 0 {
			continue
		}

		/*
			To get the fee for a given transaction the following is needed
			1. Calculate the total amount spent for the transaction using the transaction outputs
				and associated values
			2. For each transaction input
				- lookup the previous transaction id
				- fetch the transaction output at the given index
				- get the value
				- subtract this value from the total spent
			3. Remaining value is the fee
		*/
		//tx.TxIn[0].PreviousOutPoint.Hash

		// lists satoshis being spent
		//tx.TxOut[0].Value
	}
	require.NoError(t, err)
	require.NotEmpty(t, block.Transactions)
}

func TestRest_GetTransaction(t *testing.T) {
	r := NewRest()

	hash, err := chainhash.NewHashFromStr("3f9f157ee6dadfda07e809d0631831bacaf8ade4bf5461b7e3b3db7511825418")
	require.NoError(t, err)
	tx, err := r.GetTransaction(context.Background(), *hash)
	require.NoError(t, err)
	require.NotEmpty(t, tx.TxIn)
	require.NotEmpty(t, tx.TxOut)
}

func TestRest_GetMempoolTransactionIDs(t *testing.T) {
	r := NewRest()

	hashes, err := r.GetMempoolTransactionIDs(context.Background())
	t.Log(len(hashes))
	require.NoError(t, err)
	require.NotEmpty(t, hashes)
}

func TestDecodeTransaction(t *testing.T) {
	raw := hex.NewDecoder(
		strings.NewReader("0100000000010111bfcfdbf25f3b29b45a276cd0c0f4c2222ac1c8e33e8f1a7e23579a38442e760000000000fdffffff036a8a3d06000000001600141c6977423aa4b82a0d7f8496cdf3fc2f8b4f580c0778020000000000160014e70a6139bae5247d070f1f853ab3ee132b4642dd28cd01000000000016001429ad7ec6baee08cc634fcf6243bc35197feef4fb0248304502210098d0f5682848f222a2c27b8e13684420a18b24d436c3f75d0083685e7379efbe02204bc205a600bf260604b4334dabd2506ad4b1c2e5e703ee881374c234ef48ebe2012102084ad9ff2a070ef71f32375ff91e5f98448afc62bfb5934c3c15b4348cf11df700000000"),
	)
	var tx wire.MsgTx
	err := tx.Deserialize(raw)
	require.NoError(t, err)
}
