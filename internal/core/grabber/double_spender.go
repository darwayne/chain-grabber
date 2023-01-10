package grabber

import (
	"context"
	"github.com/btcsuite/btcd/btcjson"
)

type DoubleSpender struct {
	mngr   *TransactionManager
	lookup AddressMap
}

func NewDoubleSpender(mngr *TransactionManager, addressMap AddressMap) *DoubleSpender {
	return &DoubleSpender{
		mngr:   mngr,
		lookup: addressMap,
	}
}

func (d DoubleSpender) MonitorUTXOs(ctx context.Context, utxos []UTXO) chan *btcjson.TxRawResult {
	notify := make(chan *btcjson.TxRawResult, 1)

	go func() {
		channel := d.mngr.Subscribe()
		myCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-myCtx.Done():
				d.mngr.Unsubscribe(channel)
			}
		}()
		type Point struct {
			TXID  string
			Index uint32
		}
		lookup := make(map[Point]struct{})
		for _, u := range utxos {
			p := Point{
				TXID:  u.Transaction.TxHash().String(),
				Index: uint32(u.Index),
			}
			lookup[p] = struct{}{}
		}
	mainLoop:
		for info := range channel {
			for _, out := range info.Vout {
				for _, addr := range out.ScriptPubKey.Addresses {
					if d.lookup.Has(addr) {
						continue mainLoop
					}
				}
			}

			for _, in := range info.Vin {
				p := Point{
					TXID:  in.Txid,
					Index: in.Vout,
				}
				if _, found := lookup[p]; found {
					notify <- info
					continue mainLoop
				}
			}
		}
	}()

	return notify
}
