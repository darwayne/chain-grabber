package defender

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"sync"
	"time"
)

// Defender is responsible for broadcasting
// a certain transaction whenever a given input is detected
type Defender struct {
	mu                   sync.RWMutex
	transactionsToIgnore map[chainhash.Hash]TransactionInfo
	inputsToMonitor      MonitoredOutPoint
}

type MonitoredOutPoint map[chainhash.Hash]map[uint32]*chainhash.Hash

func (m MonitoredOutPoint) Add(point wire.OutPoint, hash *chainhash.Hash) {
	if m[point.Hash] == nil {
		m[point.Hash] = make(map[uint32]*chainhash.Hash)
	}
	m[point.Hash][point.Index] = hash
}

func (m MonitoredOutPoint) Remove(point wire.OutPoint) {
	if m[point.Hash] == nil {
		m[point.Hash] = make(map[uint32]*chainhash.Hash)
	}
	delete(m[point.Hash], point.Index)
	if len(m[point.Hash]) == 0 {
		delete(m, point.Hash)
	}
}

func (m MonitoredOutPoint) Hash(point wire.OutPoint) *chainhash.Hash {
	if m[point.Hash] == nil {
		return nil
	}

	for _, hash := range m[point.Hash] {
		return hash
	}

	return nil
}

type TransactionInfo struct {
	CompressedTX []byte
}

func (t TransactionInfo) Tx() (*wire.MsgTx, error) {
	buff := bytes.NewBuffer(t.CompressedTX)
	data, err := gzip.NewReader(buff)
	if err != nil {
		return nil, err
	}

	var tx wire.MsgTx
	if err := tx.Deserialize(data); err != nil {
		return nil, err
	}

	return &tx, nil
}

func New() *Defender {
	return &Defender{
		transactionsToIgnore: make(map[chainhash.Hash]TransactionInfo),
		inputsToMonitor:      make(MonitoredOutPoint),
	}
}

func (d *Defender) Defended(tx *wire.MsgTx) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.transactionsToIgnore, tx.TxHash())
	for _, in := range tx.TxIn {
		d.inputsToMonitor.Remove(in.PreviousOutPoint)
	}
}

func (d *Defender) Defend(tx *wire.MsgTx) error {
	var buff bytes.Buffer
	writer := gzip.NewWriter(&buff)
	if err := tx.Serialize(writer); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	info := TransactionInfo{
		CompressedTX: buff.Bytes(),
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	hash := tx.TxHash()
	for _, in := range tx.TxIn {
		if d.inputsToMonitor[in.PreviousOutPoint.Hash] == nil {
			d.inputsToMonitor[in.PreviousOutPoint.Hash] = make(map[uint32]*chainhash.Hash)
		}
		d.inputsToMonitor.Add(in.PreviousOutPoint, &hash)
	}
	d.transactionsToIgnore[hash] = info
	fmt.Println(hash.String(), "is being monitored", len(d.inputsToMonitor), "outpoints monitored")

	return nil
}

func (d *Defender) Start(node *lightnode.Node, broadcastClients []*broadcaster.ElectrumClient) {
	broker := broadcaster.NewBroker[*string]()
	go broker.Start()
	for idx := range broadcastClients {
		cli := broadcastClients[idx]
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			sub := broker.Subscribe()
			for {
				select {
				case <-node.Done():
					return
				case msg := <-sub:
					tx, err := cli.Broadcast(*msg)
					fmt.Println("broadcast result:", tx, err)
				case <-ticker.C:
					_, err := cli.Ping()
					if err != nil {
						fmt.Println("error pinging reconnecting to", cli.Server)
						cli.Reconnect()
					}

				}
			}
		}()
	}

	for {
		select {
		case <-node.Done():
			return
		case peer := <-node.OnPeerConnected:

			go func() {
				fmt.Println("node connected to", peer)
				data := node.GetPeerData(peer)
				if data == nil {
					fmt.Println("peer exiting early", peer)
					return
				}

				go func() {
					for {
						select {
						case <-data.Disconnected:
							fmt.Println("peer disconnected early", peer)
							return
						case <-node.Done():
							return
						case tx := <-data.OnTX:
							func() {
								txHash := tx.TxHash()
								d.mu.RLock()
								_, f := d.transactionsToIgnore[txHash]
								d.mu.RUnlock()
								if f {
									fmt.Println("skipping defended tx", txHash)
									return
								}

								//fmt.Println("got tx", tx.TxHash().String())
								for _, in := range tx.TxIn {
									d.mu.RLock()
									knownHash := d.inputsToMonitor.Hash(in.PreviousOutPoint)
									d.mu.RUnlock()
									if knownHash == nil {
										continue
									}
									fmt.Println("known input seen!", in.PreviousOutPoint.String())
									d.mu.RLock()
									info, found := d.transactionsToIgnore[*knownHash]
									d.mu.RUnlock()
									if !found {
										fmt.Println("could not find transaction with same input", in.PreviousOutPoint.String())
										continue
									}

									txToSend, err := info.Tx()
									if err != nil {
										fmt.Printf("error retrieving tx: %s\n", err)
										return
									}

									var buff bytes.Buffer
									writer := hex.NewEncoder(&buff)
									if err := txToSend.Serialize(writer); err != nil {
										fmt.Printf("error serializing tx: %s\n", err)
										return
									}

									str := buff.String()
									fmt.Println("malicious tx detected", tx.TxHash().String())
									fmt.Println("publishing", txToSend.TxHash(), "as a counter measure")
									broker.Publish(&str)
									d.Defended(txToSend)
								}
							}()

						}
					}
				}()

				for {
					select {
					case <-data.Disconnected:
						fmt.Println("peer disconnected early", peer)
						return
					case msg := <-data.OnInvoice:
						fmt.Println("received invoice from", peer)
						retrieve := wire.NewMsgGetData()
						for _, inv := range msg.InvList {
							if !(inv.Type == wire.InvTypeWitnessTx || inv.Type == wire.InvTypeTx) {
								continue
							}
							d.mu.RLock()
							info, known := d.transactionsToIgnore[inv.Hash]
							d.mu.RUnlock()
							if known {
								tx, _ := info.Tx()
								if tx != nil {
									d.Defended(tx)
								}
								continue
							}
							retrieve.AddInvVect(inv)
						}

						peer.QueueMessage(retrieve, nil)

					}
				}

			}()
		}
	}

}
