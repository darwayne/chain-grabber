package grabber

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func Hex2Wif(hexEncodedPrivKey string, params *chaincfg.Params) (string, error) {
	h, err := hex.DecodeString(hexEncodedPrivKey)
	if err != nil {
		return "", err
	}
	key, _ := btcec.PrivKeyFromBytes(h)
	wif, err := btcutil.NewWIF(key, params, true)
	if err != nil {
		return "", err
	}

	return wif.String(), nil
}
