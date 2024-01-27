package lightnode

import (
	"context"
	"encoding/hex"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestBroadcast(t *testing.T) {
	node, err := NewTestNet()
	require.NoError(t, err)

	tx := hexToTX(t, "02000000000101b4780560590c770fd4b714b56118f32b8bb5dbc227a0bbc2fc36604dc89c1fbb0000000000fdffffff02122d000000000000160014c5d97d2adbaaa3fb80ca825f9d0f9e15c6cfafe6a62e0000000000001600144762f13fe549727a8cc50810a10ad15cd988185a02473044022079f9310402002db478bec2870d6cb832ca83a65aacb7fef9121d63852193ec1a0220385d7ec1596e8643abe3a89edad53533655f5f7555a07f5c2039913303ffae8901210381a31597e6cca097b3da69a379ac3e56632c7fd30b35ea46a4005db6c2f017950f4e2700")

	workers := 400

	go func() {
		<-node.Done()
		t.FailNow()
	}()

	for i := 0; i < workers; i++ {
		go func() {
			select {
			case peer := <-node.peerConnected:
				t.Log("connected to peer", peer)
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()

					t.Log("sending transaction", tx.TxHash())
					err = node.SendTransactionToPeer(ctx, peer, tx)
					if err != nil {
						t.Log("ruh oh got an error", err, "from", peer)
					} else {
						t.Log("sent transaction!")
					}

				}()

			}
		}()
	}

	node.ConnectV2()

	select {}
}

func hexToTX(t *testing.T, hexString string) *wire.MsgTx {
	s, err := hex.DecodeString(hexString)
	require.NoError(t, err)
	if s[len(s)-1] == 0x01 {
		t.Log("main net")
	} else if s[len(s)-1] == 0x04 {
		t.Log("testnet")
	}
	var tx wire.MsgTx

	err = tx.Deserialize(hex.NewDecoder(strings.NewReader(hexString)))
	require.NoError(t, err)

	return &tx

}
