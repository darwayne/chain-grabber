package testhelpers

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TxFromHex(t *testing.T, str string) *wire.MsgTx {
	var tx wire.MsgTx
	err := tx.Deserialize(hex.NewDecoder(strings.NewReader(str)))
	require.NoError(t, err)

	return &tx
}
