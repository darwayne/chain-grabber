package transactionclassifier

import (
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"strings"
)

func From(tx *wire.MsgTx) (*chaincfg.Params, error) {
	for _, out := range tx.TxOut {
		a, b, c, d := txscript.ExtractPkScriptAddrs(out.PkScript, &chaincfg.MainNetParams)
		_, _, _, _ = a, b, c, d
		fmt.Println(b)
	}
	return nil, nil
}

func FromHex(hexString string) (*chaincfg.Params, error) {
	var tx wire.MsgTx

	err := tx.Deserialize(hex.NewDecoder(strings.NewReader(hexString)))
	if err != nil {
		return nil, err
	}

	return From(&tx)
}
