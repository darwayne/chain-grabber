package txhelper

import (
	"bytes"
	"encoding/hex"
	"github.com/btcsuite/btcd/wire"
)

func ToString(tx *wire.MsgTx) string {
	var buff bytes.Buffer
	writer := hex.NewEncoder(&buff)
	err := tx.Serialize(writer)
	if err != nil {
		return ""
	}

	return buff.String()
}

func ToStringPTR(tx *wire.MsgTx) *string {
	str := ToString(tx)
	if str == "" {
		return nil
	}

	return &str
}
