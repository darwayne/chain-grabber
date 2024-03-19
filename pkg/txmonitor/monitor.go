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
	"time"
)

type Monitor struct {
	cache  simplelru.LRUCache[chainhash.Hash, struct{}]
	broker *broadcaster.Broker[*wire.MsgTx]
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

	for {
		select {
		case <-ctx.Done():
			return
		case <-data.Disconnected:
			return
		case tx := <-data.OnTX:
			go func() {
				m.cache.Add(tx.TxHash(), struct{}{})
				m.broker.Publish(tx)
			}()

		case msg := <-data.OnInvoice:
			go func() {
				retrieve := wire.NewMsgGetData()
				for _, inv := range msg.InvList {
					if !(inv.Type == wire.InvTypeWitnessTx || inv.Type == wire.InvTypeTx) {
						continue
					}
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
