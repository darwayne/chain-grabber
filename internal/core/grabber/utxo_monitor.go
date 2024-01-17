package grabber

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type UTXOMonitor struct {
	mngr      *TransactionManager
	utxos     []UTXO
	outpoints map[wire.OutPoint]struct{}
}

func NewUTXOMonitor(mngr *TransactionManager, utxos []UTXO) *UTXOMonitor {
	outpoints := make(map[wire.OutPoint]struct{})
	for _, u := range utxos {
		outpoints[u.Outpoint] = struct{}{}
	}
	return &UTXOMonitor{
		mngr:      mngr,
		utxos:     utxos,
		outpoints: outpoints,
	}
}

func (s *UTXOMonitor) Detect() bool {
	channel := s.mngr.Subscribe()
	defer func() {
		s.mngr.Unsubscribe(channel)
	}()
	for tx := range channel {
		if tx.Confirmations != 0 {
			continue
		}
		for _, in := range tx.Vin {
			hash, _ := chainhash.NewHashFromStr(in.Txid)
			point := wire.OutPoint{Hash: *hash, Index: in.Vout}
			_, found := s.outpoints[point]
			if found {
				return true
			}
		}
	}

	return false

}
