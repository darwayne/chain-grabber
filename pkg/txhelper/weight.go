package txhelper

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
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

func CalcTxSize(tx *wire.MsgTx, secretStore txauthor.SecretsSource, prevPKScripts [][]byte, amounts []btcutil.Amount) (float64, error) {
	newTx := wire.NewMsgTx(tx.Version)
	for _, in := range tx.TxIn {
		newTx.AddTxIn(wire.NewTxIn(&in.PreviousOutPoint, in.SignatureScript, in.Witness))
	}
	for _, out := range tx.TxOut {
		newTx.AddTxOut(wire.NewTxOut(out.Value, out.PkScript))
	}

	if err := txauthor.AddAllInputScripts(newTx, prevPKScripts, amounts, secretStore); err != nil {
		return 0, err
	}

	return VBytes(newTx), nil
}
