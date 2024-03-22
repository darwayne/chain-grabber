package txhelper

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

func VBytes(tx *wire.MsgTx) float64 {
	weight := blockchain.GetTransactionWeight(btcutil.NewTx(tx))

	return float64(weight) / float64(blockchain.WitnessScaleFactor)
}

func SatsPerVByte(inputValue int64, tx *wire.MsgTx) float64 {
	fee := inputValue
	for _, out := range tx.TxOut {
		fee += -out.Value
	}

	vBytes := VBytes(tx)

	return float64(fee) / vBytes
}
