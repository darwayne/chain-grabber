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
	"golang.org/x/net/proxy"
	"log"
	"maps"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type netDialer func(network, addr string) (c net.Conn, err error)
type Node struct {
	ctx              context.Context
	dialer           proxy.Dialer
	chain            *chaincfg.Params
	peerConnected    chan *peer.Peer
	peerDisconnected chan *peer.Peer
	onPeerInv        chan PeerInv
	onPeerBlock      chan PeerBlock
	onPeerHeaders    chan PeerHeaders
	peers            map[*peer.Peer]*peerData
	mu               sync.RWMutex
	connected        bool
	connectedPeers   int64
	initialPeers     map[string]struct{}
	peerIdx          int
}

type peerData struct {
	connected      chan struct{}
	onHeaders      chan *wire.MsgHeaders
	onInvoice      chan *wire.MsgInv
	onBlock        chan *wire.MsgBlock
	sent           int64
	received       int64
	versionLatency time.Duration
	manuallySent   int64
}

func NewTestNet() (*Node, error) {
	d, err := proxy.SOCKS5("tcp", "atl.socks.ipvanish.com:1080", &proxy.Auth{
		User:     os.Getenv("PROXY_USER"),
		Password: os.Getenv("PROXY_PASS"),
	}, &net.Dialer{})
	if err != nil {
		return nil, err
	}

	return NewNode(d, &chaincfg.TestNet3Params), nil
}

func NewMainNet() (*Node, error) {
	d, err := proxy.SOCKS5("tcp",
		//"atl.socks.ipvanish.com:1080",
		"mia.socks.ipvanish.com:1080",
		&proxy.Auth{
			User:     os.Getenv("PROXY_USER"),
			Password: os.Getenv("PROXY_PASS"),
		}, &net.Dialer{})
	if err != nil {
		return nil, err
	}

	return NewNode(d, &chaincfg.MainNetParams), nil
}

func NewNode(dialer proxy.Dialer, chain *chaincfg.Params) *Node {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		select {
		case <-c:
		}
		cancel()
	}()
	return &Node{
		ctx:              ctx,
		dialer:           dialer,
		chain:            chain,
		peers:            make(map[*peer.Peer]*peerData),
		initialPeers:     make(map[string]struct{}),
		peerConnected:    make(chan *peer.Peer),
		peerDisconnected: make(chan *peer.Peer),
		onPeerBlock:      make(chan PeerBlock, 100),
		onPeerHeaders:    make(chan PeerHeaders, 100),
		onPeerInv:        make(chan PeerInv, 100),
	}
}

func (n *Node) Connect() {
	//n.mu.Lock()
	//if n.connected {
	//	n.mu.Unlock()
	//	return
	//}
	//
	//n.connected = true
	//n.mu.Unlock()
	wait := make(chan struct{}, 1)
	connmgr.SeedFromDNS(n.chain,
		wire.SFNodeNetwork, net.LookupIP, func(addrs []*wire.NetAddressV2) {
			for _, addr := range addrs {
				if strings.Contains(addr.Addr.String(), ":") {
					continue
				}

				n.mu.Lock()
				n.initialPeers[fmt.Sprintf("%s:%d", addr.Addr.String(), addr.Port)] = struct{}{}
				n.mu.Unlock()
			}
			select {
			case wait <- struct{}{}:
			default:
			}
		})

	select {
	case <-wait:
		n.connectToInitialPeers()
	case <-n.ctx.Done():
	}
}

func (n *Node) connectToInitialPeers() {
	n.mu.RLock()
	data := maps.Clone(n.initialPeers)
	n.mu.RUnlock()

	var wg sync.WaitGroup
	for addr := range data {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.connectPeer(addr)
		}()
	}

	wg.Wait()
}

func (n *Node) newestBlock() (*chainhash.Hash, int32, error) {
	return n.chain.GenesisHash, 1, nil
}

func (n *Node) GetPeerData(p *peer.Peer) *peerData {
	n.mu.RLock()
	data, found := n.peers[p]
	n.mu.RUnlock()
	if !found {
		return nil
	}

	return data
}

func (n *Node) connectPeer(addr string) (e error) {
	//defer func() {
	//	if e != nil {
	//		n.mu.Lock()
	//		delete(n.initialPeers, addr)
	//		n.mu.Unlock()
	//	}
	//}()
	data := &peerData{
		connected: make(chan struct{}),
		onHeaders: make(chan *wire.MsgHeaders, 1),
		onInvoice: make(chan *wire.MsgInv, 1),
		onBlock:   make(chan *wire.MsgBlock, 1),
	}
	var versionTime time.Time
	p, err := peer.NewOutboundPeer(&peer.Config{
		ChainParams: n.chain,
		NewestBlock: n.newestBlock,
		Listeners: peer.MessageListeners{
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				//fmt.Println("got v1 addresses", len(msg.AddrList))
				for _, a := range msg.AddrList {
					if a.IP.To4() == nil {
						continue
					}
					addr := fmt.Sprintf("%s:%d",
						a.IP.String(), a.Port)
					n.mu.RLock()
					_, found := n.initialPeers[addr]
					n.mu.RUnlock()
					if found {
						continue
					}
					n.mu.Lock()
					n.initialPeers[addr] = struct{}{}
					n.mu.Unlock()
					if atomic.LoadInt64(&n.connectedPeers) <= 1000 {
						go n.connectPeer(addr)
					}

				}
			},
			OnAddrV2: func(p *peer.Peer, msg *wire.MsgAddrV2) {
				//fmt.Println("got v2 addresses", len(msg.AddrList))
				for _, a := range msg.AddrList {
					if strings.Contains(a.Addr.String(), ":") {
						continue
					}
					addr := fmt.Sprintf("%s:%d",
						a.Addr.String(), a.Port)
					n.mu.RLock()
					_, found := n.initialPeers[addr]
					n.mu.RUnlock()
					if found {
						continue
					}
					n.mu.Lock()
					n.initialPeers[addr] = struct{}{}
					n.mu.Unlock()
					if atomic.LoadInt64(&n.connectedPeers) <= 1000 {
						go n.connectPeer(addr)
					}

				}
			},
			OnVersion: func(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
				versionTime = time.Now()
				return nil
			},
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				data.versionLatency = time.Since(versionTime)
				close(data.connected)
				select {
				case n.peerConnected <- p:
				case <-n.ctx.Done():
				default:

				}

				p.QueueMessage(&wire.MsgGetAddr{}, nil)
			},
			OnHeaders: func(p *peer.Peer, msg *wire.MsgHeaders) {
				select {
				case n.onPeerHeaders <- PeerHeaders{Peer: p, MsgHeaders: msg}:
				case <-n.ctx.Done():
				default:
				}
				select {
				case data.onHeaders <- msg:
				case <-n.ctx.Done():
				default:
				}
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				select {
				case n.onPeerInv <- PeerInv{Peer: p, MsgInv: msg}:
				case <-n.ctx.Done():
				default:
				}
				select {
				case data.onInvoice <- msg:
				case <-n.ctx.Done():
				default:
				}
			},
			OnBlock: func(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
				//log.Println("received block from", p, "hash:", msg.BlockHash())
				select {
				case n.onPeerBlock <- PeerBlock{Peer: p, MsgBlock: msg}:
					//log.Println("sent block to global channel", p, "hash:", msg.BlockHash())
				case <-n.ctx.Done():
				default:

					//log.Println("block skipped global channel", p, "hash:", msg.BlockHash())
				}
				select {
				case data.onBlock <- msg:
					//log.Println("sent block to local channel", p, "hash:", msg.BlockHash())
				case <-n.ctx.Done():
				default:
					log.Println("block skipped local channel", p, "hash:", msg.BlockHash())
				}
			},
			OnMerkleBlock: func(p *peer.Peer, msg *wire.MsgMerkleBlock) {

			},
			OnRead: func(p *peer.Peer, bytesRead int, msg wire.Message, err error) {
				//log.Println("read message", getMessageCommand(msg), "from", p)
				atomic.AddInt64(&data.received, 1)
			},
			OnWrite: func(p *peer.Peer, bytesWritten int, msg wire.Message, err error) {
				//log.Println("wrote message", getMessageCommand(msg), "to", p)
				atomic.AddInt64(&data.received, 1)
			},
		},
	}, addr)
	if err != nil {
		//log.Println("error creating outbound peer at", addr, "\nerr:", err)
		return err
	}
	conn, err := n.dialer.Dial("tcp", addr)
	if err != nil {
		//log.Println("error connecting to outbound peer at", addr, "\nerr:", err)
		return err
	}
	p.AssociateConnection(conn)
	atomic.AddInt64(&n.connectedPeers, 1)

	go func() {
		select {
		case <-n.ctx.Done():
			p.Disconnect()
		}
	}()
	go func() {
		p.WaitForDisconnect()
		atomic.AddInt64(&n.connectedPeers, -1)
		n.mu.Lock()
		delete(n.peers, p)
		n.mu.Unlock()
		select {
		case n.peerDisconnected <- p:
		case <-n.ctx.Done():
		default:
		}
	}()

	n.mu.Lock()
	n.peers[p] = data
	n.mu.Unlock()
	return nil
}

func (n *Node) TotalPeers() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.peers)
}
func (n *Node) GetPeer() *peer.Peer {
	n.mu.RLock()
	data := maps.Clone(n.peers)
	n.mu.RUnlock()
	peers := make([]*peer.Peer, 0, len(data))
	for p := range data {
		peers = append(peers, p)
	}
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].String() < peers[j].String()
	})

	n.mu.Lock()
	n.peerIdx++
	if n.peerIdx >= len(data) {
		n.peerIdx = 0
	}
	idx := n.peerIdx
	n.mu.Unlock()
	if len(peers) == 0 {
		return nil
	}

	return peers[idx]
}

func (n *Node) PopulateHeaders() {
	if 1 == 1 {
		return
		// TODO: all the logic below should live somewhere else
		// 	node should just be a node
	}
	n.mu.RLock()
	peers := maps.Clone(n.peers)
	n.mu.RUnlock()
	totalPeers := len(peers)
	fmt.Println("populating headers", len(peers), "connected")
	if totalPeers == 0 {
		n.mu.Lock()
		n.connected = false
		n.mu.Unlock()
		go func() {
			n.Connect()
			n.PopulateHeaders()
		}()

		fmt.Println("no peers detected ... re-connecting")

		return
	}

	var mu sync.RWMutex
	headerMsg := make(chan struct{})
	var blocks []chainhash.Hash //[]chainhash.Hash
	knownHashes := make(map[chainhash.Hash]struct{})
	blocks = append(blocks, *n.chain.GenesisHash)
	knownHashes[*n.chain.GenesisHash] = struct{}{}

	go func() {

		for {
			select {
			case <-headerMsg:
			case <-time.After(5 * time.Second):
				fmt.Println("no header message seen .. resending")
				mu.RLock()
				lastBlock := blocks[len(blocks)-1]
				mu.RUnlock()

				n.GetPeer().PushGetHeadersMsg(blockchain.BlockLocator{&lastBlock}, &chainhash.Hash{})
			}
		}
	}()

	folder := "./storedblocks"
	if n.chain == &chaincfg.MainNetParams {
		folder += "/mainnet/"
	} else {
		folder += "/testnet/"
	}
	folder += "blocks.sqlite"

	db, err := sql.Open("sqlite3", folder+"?journal_mode=WAL")
	if err != nil {
		log.Fatalln(err)
	}
	go func() {
		select {
		case <-n.ctx.Done():
			db.Close()
		}
	}()

	const create string = `
  CREATE TABLE IF NOT EXISTS blocks (
  hash text NOT NULL PRIMARY KEY,
  data BLOB NOT NULL
  );`

	_, err = db.Exec(create)
	if err != nil {
		log.Fatalln("error setting up sqlite", err)
	}

	blockChan := make(chan chainhash.Hash, 1)
	go func() {
		var blockMu sync.RWMutex
		blocksToFetch := make(map[chainhash.Hash]struct{})

		go func() {
			pendingBlocks := make(map[chainhash.Hash]struct{})
			myBlocks := make(map[chainhash.Hash]*wire.MsgBlock)

			//var lastPurge = time.Now()
			work := func() {

				if len(myBlocks) > 1 { //} && time.Since(lastPurge) > time.Minute {
					start := time.Now()
					fmt.Println("flushing blocks to disk")
					maxToPurge := 0
					tx, err := db.Begin()
					if err != nil {
						log.Fatalln("error opening transaction", err)
					}
					defer tx.Commit()
					stmt, err := tx.PrepareContext(n.ctx, `INSERT OR IGNORE into blocks VALUES(?, ?)`)
					if err != nil {
						log.Fatalln("error opening statement", err)
					}
					defer stmt.Close()
					for hash, block := range myBlocks {
						maxToPurge++

						err := func() error {
							f := new(bytes.Buffer)
							writer, err := gzip.NewWriterLevel(f, gzip.BestCompression)
							if err != nil {
								return err
							}

							if err := block.Serialize(writer); err != nil {
								return err
							}
							writer.Close()
							_, err = stmt.ExecContext(
								n.ctx,
								hash.String(), f.String())

							return err
						}()

						if err != nil {
							log.Fatalln("error writing block", err)
						}
						delete(myBlocks, hash)

					}

					//lastPurge = time.Now()
					log.Println(maxToPurge, "blocks flushed to disk", time.Since(start))

				}

				invoices := make([]*wire.InvVect, 0, len(pendingBlocks))

				for hash := range pendingBlocks {
					hash := hash
					invoices = append(invoices, wire.NewInvVect(wire.InvTypeBlock, &hash))
				}
				for _, chunk := range ChunkSlice(invoices, 100) {
					chunk := chunk
					data := wire.NewMsgGetData()
					data.InvList = chunk
					n.GetPeer().QueueMessage(data, nil)
				}

			}
			for {
				select {
				case block := <-n.onPeerBlock:
					hash := block.BlockHash()
					delete(pendingBlocks, hash)
					blockMu.Lock()
					delete(blocksToFetch, hash)
					blockMu.Unlock()
					myBlocks[hash] = block.MsgBlock
					//fmt.Println("got block", hash)
				case <-time.After(time.Second):
					blockMu.RLock()
					toFetch := len(blocksToFetch)
					blockMu.RUnlock()
					log.Println(len(myBlocks), "blocks saved ..", toFetch, "more remaining")
					maxPending := 20000
					if len(pendingBlocks) < maxPending {
						blockMu.RLock()
					innerLoop:
						for hash := range blocksToFetch {
							if len(pendingBlocks) < maxPending {
								pendingBlocks[hash] = struct{}{}
							} else {
								break innerLoop
							}
						}
						blockMu.RUnlock()
					}
					work()
				}
			}
		}()
		for {
			select {
			case hash := <-blockChan:
				blockMu.Lock()
				blocksToFetch[hash] = struct{}{}
				blockMu.Unlock()
			}
		}
	}()

	mu.RLock()
	lastBlock := blocks[len(blocks)-1]
	mu.RUnlock()
	n.GetPeer().PushGetHeadersMsg(blockchain.BlockLocator{&lastBlock}, &chainhash.Hash{})
	//p := n.GetPeer()
	//fmt.Println("sending blocks msg to", p.Addr())
	//p.PushGetBlocksMsg(blockchain.BlockLocator{n.chain.GenesisHash}, &chainhash.Hash{})
	go func() {
		for {
			select {
			case msg := <-n.onPeerInv:
				addr := msg.Peer.Addr()
				_ = addr

				hashes := make([]chainhash.Hash, 0, 500)
				for _, inv := range msg.InvList {
					if inv.Type != wire.InvTypeBlock {
						//fmt.Println("skipping", inv.Type, addr)
						continue
					}
					//n.GetPeer().PushGetHeadersMsg()
					hashes = append(hashes, inv.Hash)
				}

				if len(hashes) > 0 {
					//fmt.Println("got peer invoice", len(msg.InvList), addr, len(hashes))
					n.GetPeer().PushGetHeadersMsg(blockchain.BlockLocator{&hashes[0]}, &chainhash.Hash{})
				}
			case msg := <-n.onPeerHeaders:
				select {
				case headerMsg <- struct{}{}:
				default:

				}
				for _, header := range msg.Headers {
					hash := header.BlockHash()

					_, found := knownHashes[hash]
					_, valid := knownHashes[header.PrevBlock]
					if found || !valid {
						//fmt.Println("skipping known or invalid hash", hash)
						continue
					}

					mu.Lock()
					blocks = append(blocks, hash)
					mu.Unlock()
					knownHashes[hash] = struct{}{}
					blockChan <- hash
				}
				mu.RLock()
				lastIdx := len(blocks) - 1
				mu.RUnlock()
				lastBlock := blocks[lastIdx]

				//fmt.Println(lastIdx+1, "total blocks")
				n.GetPeer().PushGetHeadersMsg(blockchain.BlockLocator{&lastBlock}, &chainhash.Hash{})

			}
		}
	}()

}

type PeerInv struct {
	Peer *peer.Peer
	*wire.MsgInv
}

type PeerHeaders struct {
	Peer *peer.Peer
	*wire.MsgHeaders
}

type PeerBlock struct {
	Peer *peer.Peer
	*wire.MsgBlock
}

func (n *Node) Done() <-chan struct{} {
	return n.ctx.Done()
}

func getMessageCommand(msg wire.Message) string {
	if msg == nil {
		return ""
	}
	return msg.Command()
}

func ChunkSlice[T any](items []T, chunkSize int) (chunks [][]T) {
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}
	return append(chunks, items)
}
