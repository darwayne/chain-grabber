package txmonitor

import (
	"context"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"sync"
	"sync/atomic"
	"time"
)

type Monitor struct {
	mu        sync.Mutex
	cache     simplelru.LRUCache[chainhash.Hash, struct{}]
	broker    *broadcaster.Broker[*wire.MsgTx]
	connected int64

	queueMu sync.RWMutex
	queue   []*wire.MsgTx
}

func New() *Monitor {
	b := broadcaster.NewBroker[*wire.MsgTx]()
	go b.Start()
	return &Monitor{
		cache:  expirable.NewLRU[chainhash.Hash, struct{}](5_000, nil, 5*time.Minute),
		broker: b,
	}
}

func (m *Monitor) Subscribe() chan *wire.MsgTx {
	return m.broker.Subscribe()
}

func (m *Monitor) UnSubscribe(channel chan *wire.MsgTx) {
	m.broker.UnSubscribe(channel)
}

func (m *Monitor) Stop() {
	m.broker.Stop()
}

func (m *Monitor) Start(ctx context.Context, node *lightnode.Node) {
	go func() {
		ticker := time.NewTicker(time.Millisecond)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			m.queueMu.RLock()
			size := len(m.queue)
			txs := make([]*wire.MsgTx, 0)
			txs = append(txs, m.queue...)
			m.queueMu.RUnlock()
			if size == 0 {
				continue
			}

			for _, tx := range txs {
				hash := tx.TxHash()

				if m.cache.Contains(hash) {
					continue
				}

				m.cache.Add(hash, struct{}{})
				m.broker.Publish(tx)
			}

			m.queueMu.Lock()
			m.queue = m.queue[size:]
			m.queueMu.Unlock()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-node.Done():
			return
		case peer := <-node.OnPeerConnected:
			go m.onPeerConnected(ctx, node, peer)
		}
	}
}

func (m *Monitor) onPeerConnected(ctx context.Context, node *lightnode.Node, peer *peer.Peer) {
	data := node.GetPeerData(peer)
	if data == nil {
		return
	}
	total := atomic.AddInt64(&m.connected, 1)
	//fmt.Println("peer connected", peer, ".. total connected", total)

	for {
		select {
		case <-ctx.Done():
			return
		case <-data.Disconnected:
			total = atomic.AddInt64(&m.connected, -1)
			_ = total
			//fmt.Println("peer disconnected", peer, ".. total connected", total)
			return
		case tx := <-data.OnTX:
			m.queueMu.Lock()
			m.queue = append(m.queue, tx)
			m.queueMu.Unlock()

		case msg := <-data.OnInvoice:
			go func() {
				retrieve := wire.NewMsgGetData()
				for _, inv := range msg.InvList {
					if !(inv.Type == wire.InvTypeWitnessTx || inv.Type == wire.InvTypeTx) {
						continue
					}
					inv.Type = wire.InvTypeWitnessTx
					if m.cache.Contains(inv.Hash) {
						continue
					}
					retrieve.AddInvVect(inv)
				}
				if len(retrieve.InvList) > 0 {
					peer.QueueMessage(retrieve, nil)
				}
			}()

		}
	}
}
