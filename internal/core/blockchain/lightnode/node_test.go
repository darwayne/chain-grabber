package lightnode

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/proxy"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlockStore(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	ctx := context.Background()
	t.Log(store.GetTip(ctx))
}

func TestSQLITECopyHeightToBlockDB(t *testing.T) {
	ddb, err := sql.Open("sqlite3", "./storedblocks/mainnet/blocks.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		ddb.Close()
	})

	const create string = `
  CREATE TABLE IF NOT EXISTS height (
  height UNSIGNED BIG INT NOT NULL PRIMARY KEY,
  hash text NOT NULL
  );`

	_, err = ddb.Exec(create)
	require.NoError(t, err)

	ctx := context.Background()
	db, err := ddb.Conn(ctx)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `ATTACH './storedblocks/mainnet/height.sqlite' as hdb`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
INSERT OR IGNORE INTO height (height, hash)
select height, hash from hdb.block_height`)
	require.NoError(t, err)

}

func TestSQLITEMissingBlocks(t *testing.T) {
	ddb, err := sql.Open("sqlite3", "./storedblocks/mainnet/height.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		ddb.Close()
	})

	ctx := context.Background()
	db, err := ddb.Conn(ctx)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `ATTACH './storedblocks/mainnet/blocks.sqlite' as bdb`)
	require.NoError(t, err)

	rows, err := db.QueryContext(ctx, `
select height, bh.hash, b.hash from block_height bh
LEFT JOIN bdb.blocks b ON b.hash = bh.hash
WHERE height < 400000 AND b.hash IS NOT NULL
ORDER BY height asc`)
	require.NoError(t, err)
	for rows.Next() {
		var height int
		var hash string
		var missingHash *string
		err = rows.Scan(&height, &hash, &missingHash)
		require.NoError(t, err)
		t.Log(height, hash, missingHash)
	}
}

func TestSQLITEBlockRead(t *testing.T) {
	ddb, err := sql.Open("sqlite3", "./storedblocks/mainnet/blocks.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		ddb.Close()
	})

	ctx := context.Background()
	db, err := ddb.Conn(ctx)
	require.NoError(t, err)

	row := db.QueryRowContext(ctx, `select data from blocks WHERE hash = ?`, "0000000000000000000060e32d547b6ae2ded52aadbc6310808e4ae42b08cc6a")
	require.NoError(t, row.Err())
	var data []byte
	require.NoError(t, row.Scan(&data))
	buff := bytes.NewBuffer(data)
	reader, err := gzip.NewReader(buff)
	require.NoError(t, err)
	var block wire.MsgBlock
	require.NoError(t, block.Deserialize(reader))

	t.Log(len(block.Transactions))
}

func TestTestNet(t *testing.T) {
	node, err := NewTestNet()
	require.NoError(t, err)

	go func() {
		for {
			select {
			case peer := <-node.peerConnected:
				data := node.GetPeerData(peer)
				fmt.Println("node connected", atomic.LoadInt64(&node.connectedPeers),
					"\tversionLatency:", data.versionLatency)
			}
		}
	}()
	node.Connect()
	require.NotEmpty(t, node.initialPeers)

	node.PopulateHeaders()
	select {
	case <-node.Done():
		time.Sleep(time.Second)
	}
}

func TestChunkSlice(t *testing.T) {
	var arr []int
	for i := 0; i < 100; i++ {
		arr = append(arr, i)
	}

	chunked := ChunkSlice(arr, 50)
	length := len(chunked)
	require.Equal(t, 2, length)
}

func TestMainNet(t *testing.T) {
	node, err := NewMainNet()
	require.NoError(t, err)
	go func() {
		for {
			select {
			case peer := <-node.peerConnected:
				data := node.GetPeerData(peer)
				fmt.Println("node connected", atomic.LoadInt64(&node.connectedPeers),
					"\tversionLatency:", data.versionLatency)
			}
		}
	}()
	node.Connect()
	require.NotEmpty(t, node.initialPeers)

	node.PopulateHeaders()
	select {
	case <-node.Done():
		time.Sleep(time.Second)
	}
}

func TestNet(t *testing.T) {
	peerAddrChan := make(chan string)
	btcChain := &chaincfg.TestNet3Params
	connmgr.SeedFromDNS(btcChain,
		wire.SFNodeNetwork|wire.SFNodeGetUTXO, net.LookupIP, func(addrs []*wire.NetAddressV2) {
			for _, addr := range addrs {
				if strings.Contains(addr.Addr.String(), ":") {
					continue
				}

				select {
				case peerAddrChan <- fmt.Sprintf("%s:%d", addr.Addr.String(), addr.Port):
				default:
					continue
				}

			}
		})

	peerAddr := <-peerAddrChan
	d, err := proxy.SOCKS5("tcp", "atl.socks.ipvanish.com:1080", &proxy.Auth{
		User:     os.Getenv("PROXY_USER"),
		Password: os.Getenv("PROXY_PASS"),
	}, &net.Dialer{})
	require.NoError(t, err)
	_ = d

	doneChan := make(chan struct{})
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		select {
		case <-c:
		}
		close(doneChan)
	}()
	connected := make(chan struct{})

	p, err := peer.NewOutboundPeer(&peer.Config{
		//Services:    wire.SFNodeNetwork | wire.SFNodeNetworkLimited,
		ChainParams: btcChain,
		NewestBlock: func() (hash *chainhash.Hash, height int32, err error) {
			return btcChain.GenesisHash, 1, nil
			//return initialHash, 643000, nil
		},
		Listeners: peer.MessageListeners{
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				connected <- struct{}{}

			},
			OnVersion: func(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {

				t.Log("got version")
				t.Log("peer supports the following services", p.Services())

				return nil
			},
			OnGetHeaders: func(p *peer.Peer, msg *wire.MsgGetHeaders) {
				t.Log("received get headers")
				for idx, hash := range msg.BlockLocatorHashes {
					t.Log("locator idx:", idx, "\thash:", hash.String())
				}

				t.Log("hash stop is", msg.HashStop.String())
			},
			OnHeaders: func(p *peer.Peer, msg *wire.MsgHeaders) {
				fmt.Println("got headers!")
				for idx, header := range msg.Headers {
					t.Log("idx", idx, "hash", header.BlockHash())
				}
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				t.Log("got an invoice")

				for idx, inv := range msg.InvList {
					t.Log("invoice type:", inv.Type)
					if inv.Type == wire.InvTypeBlock {
						_ = fmt.Sprintf("")
						log.Println("block inv idx:", idx, inv.Hash.String())
					}
				}
			},
			OnBlock: func(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
				t.Log("received block", msg.BlockHash())
			},
			OnRead: func(p *peer.Peer, bytesRead int, msg wire.Message, err error) {
				var cmd string
				if bytesRead > 0 && msg != nil {
					cmd = msg.Command()
				}
				fmt.Println("received message from peer", bytesRead, cmd, msg)
			},
			OnWrite: func(p *peer.Peer, bytesWritten int, msg wire.Message, err error) {
				var cmd string
				if bytesWritten > 0 && msg != nil {
					cmd = msg.Command()
				}
				fmt.Println("sent message to peer", bytesWritten, cmd, msg)
			},
		},
	}, peerAddr)

	require.NoError(t, err)
	_ = p
	conn, err := d.Dial("tcp", peerAddr)
	require.NoError(t, err)
	p.AssociateConnection(conn)

	select {
	case <-doneChan:
		return
	case <-connected:
	}

	err = p.PushGetHeadersMsg(blockchain.BlockLocator{getHash(t, "00000000a2424460c992803ed44cfe0c0333e91af04fde9a6a97b468bf1b5f70")}, &chainhash.Hash{})
	require.NoError(t, err)

	err = p.PushGetBlocksMsg(blockchain.BlockLocator{getHash(t, "00000000a2424460c992803ed44cfe0c0333e91af04fde9a6a97b468bf1b5f70")}, &chainhash.Hash{})
	require.NoError(t, err)

	select {
	case <-doneChan:
	}
}

func TestMeOut(t *testing.T) {
	addr := "10.0.0.37:8333"

	connected := make(chan struct{}, 1)
	var sent time.Time

	//initialHash := getHash(t, "0000000000000000000912c1736fbf6c64177497305fcafcee6f28aa0a414e17")
	p, err := peer.NewOutboundPeer(&peer.Config{
		//Services:    wire.SFNodeWitness, // | wire.SFNodeNetworkLimited,
		ChainParams: &chaincfg.MainNetParams,
		NewestBlock: func() (hash *chainhash.Hash, height int32, err error) {
			return chaincfg.MainNetParams.GenesisHash, 1, nil
			//return initialHash, 643000, nil
		},
		Listeners: peer.MessageListeners{
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				connected <- struct{}{}

			},
			OnVersion: func(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {

				t.Log("got version")
				t.Log("peer supports the following services", p.Services())

				return nil
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				t.Log("got an invoice")

				for _, inv := range msg.InvList {
					if inv.Type == wire.InvTypeBlock {
						msg := wire.NewMsgGetData()
						msg.AddInvVect(inv)
						p.QueueMessage(msg, nil)
					}
				}
			},
			OnBlock: func(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
				t.Log("received block", msg.BlockHash(), "in", time.Since(sent))
			},
			OnRead: func(p *peer.Peer, bytesRead int, msg wire.Message, err error) {
				var cmd string
				if bytesRead > 0 && msg != nil {
					cmd = msg.Command()
				}
				fmt.Println("received message from peer", bytesRead, cmd, msg)
			},
			OnWrite: func(p *peer.Peer, bytesWritten int, msg wire.Message, err error) {
				var cmd string
				if bytesWritten > 0 && msg != nil {
					cmd = msg.Command()
				}
				fmt.Println("sent message to peer", bytesWritten, cmd, msg)
			},
		},
	}, addr)
	require.NoError(t, err)
	_ = p
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	p.AssociateConnection(conn)

	doneChan := make(chan struct{})
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		select {
		case <-c:
			p.Disconnect()
		}
		close(doneChan)
	}()

	select {
	case <-connected:
	case <-doneChan:
		return
	}
	sent = time.Now()
	fmt.Println("connected .. sending block request")
	//msg := wire.NewMsgGetData()
	//msg.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, getHash(t, "0000000000000000000912c1736fbf6c64177497305fcafcee6f28aa0a414e17")))
	//p.QueueMessage(msg, nil)

	startBlockHash := chaincfg.MainNetParams.GenesisHash

	//Send getblocks message to request blocks
	getBlocksMsg := wire.NewMsgGetBlocks(startBlockHash)
	getBlocksMsg.AddBlockLocatorHash(startBlockHash)
	p.QueueMessage(getBlocksMsg, nil)
	<-doneChan

}

func getHash(t *testing.T, str string) *chainhash.Hash {
	h, err := chainhash.NewHashFromStr(str)
	require.NoError(t, err)
	return h
}
