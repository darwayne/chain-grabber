package lightnode

import (
	"context"
	"encoding/hex"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestTwo(t *testing.T) {
	cli := broadcaster.NewElectrumClient()
	server := "xtrum.com:50002"
	err := cli.Connect(server)
	t.Cleanup(func() {
		cli.Disconnect()
	})
	require.NoError(t, err)
	t.Log(cli.Ping())
	peers, err := cli.GetPeers()
	require.NoError(t, err)
	t.Logf("%+v", peers)
	var connections []*broadcaster.ElectrumClient
	for _, p := range peers {
		if p.SSLServer() == server {
			continue
		}
		conn, err := p.Connect()
		if err != nil {
			continue
		}
		latency, _ := conn.Ping()
		t.Log(p.Hostname, latency)
		peers, err := conn.GetPeers()
		require.NoError(t, err)
		t.Logf("%+v", peers)

		connections = append(connections, conn)
	}

	//transactionHex := "0200000000010180f5fb38b0eb2bfdd3db487a760ec4d49a932fb531b85763847624fb46938a6f0100000000fdffffff02ff08230100000000160014631a5ca7989d82c412550563e450091209bc14155946e6070000000016001414a9e0d117339dc8254357186f40e96c0291421202473044022005389903097407f316c00859770b2ac52e13af62da06bc3004a6e3c00f80847f0220584dbf083b5bdb8451ca02d48e912267ea4ee84b86cd4ff7eae82f02c14c115501210377a5574b031210bd507ac7f42177c456822067e4de51c172b0c437c4bcea7b49b34e2700" // Your transaction hex
	//result, err := cli.Broadcast(transactionHex)
	//t.Log(result, err)
}

func TestBroadcast(t *testing.T) {
	node, err := NewTestNet()
	require.NoError(t, err)
	node.MaxPeers = 500

	tx := hexToTX(t, "0200000000010123beb63aea947f1699f74b67fe6b48d7b46c7989e81d4eeabdb0d28e8e6f7c760000000000fdffffff024f16010000000000160014846736ca34ccc4b210e0a8358f5305a214b5e32ae54f09090000000016001488193e9c4caf8dfe087f980737621b3d33adb53902473044022031832875f5567b76c3b5d1c9febadd8b2bfb0b4f05dab5094b54dd89d6462fe8022006e02eaea7ea8d83ea38daa5256bab245f0d16abea49012ea9b34fe68d13240c0121036a545f9aa26b0a85fa26844814fba7b82dd8bde2ec917deffe5f506f4da50ae5434e2700")

	workers := 400

	for i := 0; i < workers; i++ {
		go func() {
			select {
			case peer := <-node.OnPeerConnected:
				t.Log("connected to peer", peer)
				func() {
					//if 1 == 1 {
					//	return
					//}
					// give app 30 seconds to catch up to headers
					time.Sleep(30 * time.Second)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()

					t.Log("sending transaction to", peer, tx.TxHash())
					err = node.SendTransactionToPeer(ctx, peer, tx)
					if err != nil {
						t.Log("ruh oh got an error", err, "from", peer)
					} else {
						t.Log("sent transaction!", peer)
					}

				}()

			}
		}()
	}

	//go node.SynchronizeHeaders()
	go node.ConnectV2()

	select {
	case <-node.Done():
	}
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
