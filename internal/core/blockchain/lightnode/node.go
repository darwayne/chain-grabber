package lightnode

import (
	"context"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/darwayne/errutil"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	"maps"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Node struct {
	cancel             context.CancelFunc
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
	logger             *zap.Logger
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

func NewTestNet(logger *zap.Logger) (*Node, error) {
	var err error
	var d proxy.Dialer
	if os.Getenv("PROXY_USER") != "" {
		d, err = proxy.SOCKS5("tcp", "atl.socks.ipvanish.com:1080", &proxy.Auth{
			User:     os.Getenv("PROXY_USER"),
			Password: os.Getenv("PROXY_PASS"),
		}, &net.Dialer{})
	} else {
		d = &net.Dialer{}
	}
	if err != nil {
		return nil, err
	}

	return NewNode(logger, d, &chaincfg.TestNet3Params), nil
}

func NewMainNet(logger *zap.Logger) (*Node, error) {
	var err error
	var d proxy.Dialer
	if os.Getenv("PROXY_USER") != "" {
		d, err = proxy.SOCKS5("tcp",
			//"atl.socks.ipvanish.com:1080",
			"mia.socks.ipvanish.com:1080",
			&proxy.Auth{
				User:     os.Getenv("PROXY_USER"),
				Password: os.Getenv("PROXY_PASS"),
			}, &net.Dialer{})
	} else {
		d = &net.Dialer{}
	}

	if err != nil {
		return nil, err
	}

	return NewNode(logger, d, &chaincfg.MainNetParams), nil
}

func NewNode(logger *zap.Logger, dialer proxy.Dialer, chain *chaincfg.Params) *Node {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigutil.Wait()
		cancel()
	}()
	return &Node{
		logger:             logger,
		cancel:             cancel,
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
			//log.Println("got", len(addrs), "from dns")
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
				//log.Println(inv.Type, inv.Hash, "inv requested from", peer)
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

func (n *Node) Shutdown() {
	n.cancel()
}

func (n *Node) connectPeer(addr string) (e error) {
	atomic.AddInt64(&n.connectedPeers, 1)
	if n.TotalPeers() > int64(n.MaxPeers) && n.MaxPeers > 0 {
		atomic.AddInt64(&n.connectedPeers, -1)
		return
	}

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
		OnTX:         make(chan *wire.MsgTx, 100),
	}
	var versionTime time.Time
	p, err := peer.NewOutboundPeer(&peer.Config{
		ChainParams:     n.chain,
		TrickleInterval: 500 * time.Millisecond,
		Services:        wire.SFNodeNetwork | wire.SFNodeWitness | wire.SFNode2X,
		ProtocolVersion: peer.MaxProtocolVersion,
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
					n.logger.Warn("tx message dropped")
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
					n.logger.Warn("skipped local headers message", zap.String("peer", p.String()))
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
					n.logger.Warn("warning invoice dropped")
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
					//log.Println("block skipped local channel", p, "hash:", msg.BlockHash())
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
