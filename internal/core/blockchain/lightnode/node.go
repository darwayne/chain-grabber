package lightnode

import (
	"context"
	"fmt"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/errutil"
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

type Node struct {
	ctx                context.Context
	dialer             proxy.Dialer
	chain              *chaincfg.Params
	OnPeerConnected    chan *peer.Peer
	OnPeerDisconnected chan *peer.Peer
	OnPeerInv          chan PeerInv
	OnPeerBlock        chan PeerBlock
	OnPeerHeaders      chan PeerHeaders
	peers              map[*peer.Peer]*PeerData
	mu                 sync.RWMutex
	connected          bool
	connectedPeers     int64
	initialPeers       map[string]struct{}
	peerIdx            int
	MaxPeers           int
	headerMu           sync.RWMutex
	Headers            []wire.BlockHeader
	MaxHeaders         int
}

type PeerData struct {
	Connected        chan struct{}
	Disconnected     chan struct{}
	OnHeaders        chan *wire.MsgHeaders
	OnGetHeaders     chan *wire.MsgGetHeaders
	OnInvoice        chan *wire.MsgInv
	OnTX             chan *wire.MsgTx
	OnBlock          chan *wire.MsgBlock
	OnGetData        chan *wire.MsgGetData
	MessagesSent     int64
	MessagesReceived int64
	VersionLatency   time.Duration
	manuallySent     int64
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
		ctx:                ctx,
		dialer:             dialer,
		chain:              chain,
		peers:              make(map[*peer.Peer]*PeerData),
		initialPeers:       make(map[string]struct{}),
		OnPeerConnected:    make(chan *peer.Peer, 100),
		OnPeerDisconnected: make(chan *peer.Peer, 100),
		OnPeerBlock:        make(chan PeerBlock, 100),
		OnPeerHeaders:      make(chan PeerHeaders, 100),
		OnPeerInv:          make(chan PeerInv, 100),
	}
}

func (n *Node) ConnectV2() {
	connmgr.SeedFromDNS(n.chain,
		wire.SFNodeNetwork, net.LookupIP, func(addrs []*wire.NetAddressV2) {
			log.Println("got", len(addrs), "from dns")
			for _, addr := range addrs {
				if strings.Contains(addr.Addr.String(), ":") {
					//log.Println("skipping address", addr)
					//continue
				}

				//log.Println("adding", addr.Addr, "to list")

				connectionAddr := fmt.Sprintf("%s:%d", addr.Addr.String(), addr.Port)
				go n.connectPeer(connectionAddr)
			}
			//n.ConnectV2()
		})
}

func (n *Node) Connect() {
	//n.mu.Lock()
	//if n.Connected {
	//	n.mu.Unlock()
	//	return
	//}
	//
	//n.Connected = true
	//n.mu.Unlock()
	wait := make(chan struct{}, 1)
	connmgr.SeedFromDNS(n.chain,
		wire.SFNodeNetwork, net.LookupIP, func(addrs []*wire.NetAddressV2) {
			for _, addr := range addrs {
				if strings.Contains(addr.Addr.String(), ":") {
					log.Println("skipping address", addr)
					continue
				}

				log.Println("adding", addr.Addr, "to list")

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

func (n *Node) GetPeerData(p *peer.Peer) *PeerData {
	n.mu.RLock()
	data, found := n.peers[p]
	n.mu.RUnlock()
	if !found {
		return nil
	}

	return data
}

func (n *Node) SendTransactionToPeer(ctx context.Context, peer *peer.Peer, tx *wire.MsgTx) error {
	info := n.GetPeerData(peer)
	if info == nil {
		return errutil.NewNotFound("peer data not found")
	}
	var msg wire.MsgInv

	hash := tx.TxHash()
	err := msg.AddInvVect(wire.NewInvVect(wire.InvTypeWitnessTx, &hash))
	if err != nil {
		return err
	}
	peer.QueueMessage(&msg, nil)
	// wait for getData message that matches my hash
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req := <-info.OnGetData:
			for _, inv := range req.InvList {
				log.Println(inv.Type, inv.Hash, "inv requested from", peer)
				if !((inv.Type == wire.InvTypeTx || inv.Type == wire.InvTypeWitnessTx) && inv.Hash == hash) {
					continue
				}
				peer.QueueMessage(tx, nil)
				return nil
			}
		}
	}
}

func (n *Node) TotalPeers() int64 {
	return atomic.LoadInt64(&n.connectedPeers)
}

func (n *Node) handeHeaders(p *peer.Peer) {
	data := n.GetPeerData(p)
	if data == nil {
		log.Println("no data detected")
		return
	}
	maxHeaders := n.MaxHeaders
	if maxHeaders == 0 {
		maxHeaders = 100
	}

	n.headerMu.RLock()
	headerSize := len(n.Headers)
	n.headerMu.RUnlock()
	if headerSize == 0 {
		var hash *chainhash.Hash
		if n.chain == &chaincfg.MainNetParams {
			hash, _ = chainhash.NewHashFromStr("0000000000000000000393398b854ef335316e882ba041cf5a10013ec7b7de22")
		} else {
			hash, _ = chainhash.NewHashFromStr("0000000000000014e2245b613b729216524228d84f01e62ee17f602ea2654c2c")
		}
		p.PushGetHeadersMsg(blockchain.BlockLocator{hash}, &chainhash.Hash{})
	}

	for {
		select {
		case <-n.Done():
			return
		case <-data.Disconnected:
			return
		case req := <-data.OnBlock:
			fmt.Println("got block", req.BlockHash())
			n.headerMu.Lock()
			if len(n.Headers) == 0 {
				n.headerMu.Unlock()
				continue
			}

			lastHeader := n.Headers[len(n.Headers)-1]
			if req.Header.PrevBlock == lastHeader.BlockHash() {
				n.Headers = append(n.Headers, req.Header)
			}
			if len(n.Headers) > maxHeaders {
				fmt.Println("adding block to headers")
				n.Headers = n.Headers[len(n.Headers)-maxHeaders:]
			}
			n.headerMu.Unlock()
		case req := <-data.OnGetHeaders:
			n.headerMu.RLock()
			copied := make([]*wire.BlockHeader, 0, len(n.Headers))
			for _, header := range n.Headers {
				header := header
				copied = append(copied, &header)
			}
			n.headerMu.RUnlock()

			msg := wire.NewMsgHeaders()
			startFrom := 0
			stopAt := len(copied)
			for idx, c := range copied {
				if len(req.BlockLocatorHashes) == 0 {
					break
				}
				hash := req.BlockLocatorHashes[0]
				if *hash == c.PrevBlock {
					startFrom = idx
				}
				if req.HashStop == c.BlockHash() {
					stopAt = idx
					break
				}
			}

			for _, c := range copied[startFrom:stopAt] {
				msg.AddBlockHeader(c)
			}

			p.QueueMessage(msg, nil)

		case req := <-data.OnHeaders:
			if len(req.Headers) == 0 {
				continue
			}
			n.headerMu.Lock()
			var updated bool
			if len(n.Headers) == 0 {
				n.Headers = append(n.Headers, *req.Headers[0])
				updated = true
			}

			lastHeader := n.Headers[len(n.Headers)-1]
			for _, header := range req.Headers {
				if header.PrevBlock != lastHeader.BlockHash() {
					continue
				}

				n.Headers = append(n.Headers, *header)
				updated = true
				lastHeader = n.Headers[len(n.Headers)-1]
			}
			if len(n.Headers) > maxHeaders {
				n.Headers = n.Headers[len(n.Headers)-maxHeaders:]
			}
			total := len(n.Headers)
			n.headerMu.Unlock()
			if updated {
				fmt.Println("headers updated", total)
				hash := lastHeader.BlockHash()
				p.PushGetHeadersMsg(blockchain.BlockLocator{&hash}, &chainhash.Hash{})
			}

		}
	}
}

func (n *Node) connectPeer(addr string) (e error) {
	atomic.AddInt64(&n.connectedPeers, 1)
	if n.TotalPeers() > int64(n.MaxPeers) && n.MaxPeers > 0 {
		atomic.AddInt64(&n.connectedPeers, -1)
		return
	}
	//defer func() {
	//	if e != nil {
	//		n.mu.Lock()
	//		delete(n.initialPeers, addr)
	//		n.mu.Unlock()
	//	}
	//}()
	n.mu.Lock()
	if _, found := n.initialPeers[addr]; found {
		n.mu.Unlock()
		return
	}
	n.initialPeers[addr] = struct{}{}
	n.mu.Unlock()
	data := &PeerData{
		Connected:    make(chan struct{}),
		Disconnected: make(chan struct{}),
		OnHeaders:    make(chan *wire.MsgHeaders, 1),
		OnInvoice:    make(chan *wire.MsgInv, 1),
		OnBlock:      make(chan *wire.MsgBlock, 1),
		OnGetData:    make(chan *wire.MsgGetData, 1),
		OnGetHeaders: make(chan *wire.MsgGetHeaders, 1),
		OnTX:         make(chan *wire.MsgTx, 1),
	}
	var versionTime time.Time
	p, err := peer.NewOutboundPeer(&peer.Config{
		ChainParams:     n.chain,
		TrickleInterval: 500 * time.Millisecond,
		//NewestBlock: n.newestBlock,
		Services: wire.SFNodeNetwork,
		Listeners: peer.MessageListeners{
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				//log.Println("got v1 addresses", len(msg.AddrList))
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
			OnGetData: func(p *peer.Peer, msg *wire.MsgGetData) {
				select {
				case data.OnGetData <- msg:
				default:

				}
			},
			OnAddrV2: func(p *peer.Peer, msg *wire.MsgAddrV2) {
				//log.Println("got v2 addresses", len(msg.AddrList))
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
			OnTx: func(p *peer.Peer, msg *wire.MsgTx) {
				select {
				case data.OnTX <- msg:
				default:

				}
				//log.Println("got transaction", msg.TxHash())
			},
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				data.VersionLatency = time.Since(versionTime)
				close(data.Connected)
				select {
				case n.OnPeerConnected <- p:
				case <-n.ctx.Done():
				default:

				}

				go n.handeHeaders(p)

				p.QueueMessage(&wire.MsgGetAddr{}, nil)
			},
			OnHeaders: func(p *peer.Peer, msg *wire.MsgHeaders) {
				select {
				case n.OnPeerHeaders <- PeerHeaders{Peer: p, MsgHeaders: msg}:
				case <-n.ctx.Done():
				default:
				}
				select {
				case data.OnHeaders <- msg:
				case <-n.ctx.Done():
				default:
					log.Println("skipped local headers message", p)
				}
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				//log.Println(len(msg.InvList), "invoices received from", p)
				//go func() {
				//	for _, inv := range msg.InvList {
				//		log.Println(inv.Type, inv.Hash, "invoice from", p)
				//	}
				//}()
				select {
				case n.OnPeerInv <- PeerInv{Peer: p, MsgInv: msg}:
				case <-n.ctx.Done():
				default:
				}
				select {
				case data.OnInvoice <- msg:
				case <-n.ctx.Done():
				default:
				}
			},
			OnGetHeaders: func(p *peer.Peer, msg *wire.MsgGetHeaders) {
				select {
				case data.OnGetHeaders <- msg:
				case <-n.Done():
				default:
				}
			},
			OnSendHeaders: func(p *peer.Peer, msg *wire.MsgSendHeaders) {
				//log.Println("wants my latest headers", p)
			},
			OnBlock: func(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
				//log.Println("received block from", p, "hash:", msg.BlockHash())
				select {
				case n.OnPeerBlock <- PeerBlock{Peer: p, MsgBlock: msg}:
					//log.Println("sent block to global channel", p, "hash:", msg.BlockHash())
				case <-n.ctx.Done():
				default:

					//log.Println("block skipped global channel", p, "hash:", msg.BlockHash())
				}
				select {
				case data.OnBlock <- msg:
					//log.Println("sent block to local channel", p, "hash:", msg.BlockHash())
				case <-n.ctx.Done():
				default:
					log.Println("block skipped local channel", p, "hash:", msg.BlockHash())
				}
			},
			OnMerkleBlock: func(p *peer.Peer, msg *wire.MsgMerkleBlock) {

			},
			OnReject: func(p *peer.Peer, msg *wire.MsgReject) {
				//log.Println("got reject of", msg.Hash, msg.Reason)
			},
			OnRead: func(p *peer.Peer, bytesRead int, msg wire.Message, err error) {
				//log.Println("read message", getMessageCommand(msg), "from", p)
				atomic.AddInt64(&data.MessagesReceived, 1)
			},
			OnWrite: func(p *peer.Peer, bytesWritten int, msg wire.Message, err error) {
				//log.Println("wrote message", getMessageCommand(msg), "to", p)
				atomic.AddInt64(&data.MessagesReceived, 1)
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

	n.mu.Lock()
	n.peers[p] = data
	n.mu.Unlock()

	p.AssociateConnection(conn)

	go func() {
		select {
		case <-n.ctx.Done():
			p.Disconnect()
		}
	}()
	go func() {
		p.WaitForDisconnect()
		close(data.Disconnected)
		atomic.AddInt64(&n.connectedPeers, -1)
		n.mu.Lock()
		delete(n.peers, p)
		n.mu.Unlock()
		select {
		case n.OnPeerDisconnected <- p:
		case <-n.ctx.Done():
		default:
		}
	}()

	return nil
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
