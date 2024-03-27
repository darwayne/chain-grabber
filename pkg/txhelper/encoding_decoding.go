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

func FromString(str string) *wire.MsgTx {
	data, err := hex.DecodeString(str)
	if err != nil {
		return nil
	}

	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(data)); err != nil {
		return nil
	}

	return &tx
}
