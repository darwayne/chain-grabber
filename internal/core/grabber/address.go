package grabber

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func PrivToPubKeyHash(key *btcec.PrivateKey, compressed bool, params *chaincfg.Params) (*btcutil.AddressPubKeyHash, error) {
	var raw []byte
	if compressed {
		raw = key.PubKey().SerializeCompressed()
	} else {
		raw = key.PubKey().SerializeUncompressed()
	}
	pkHash := btcutil.Hash160(raw)
	return btcutil.NewAddressPubKeyHash(pkHash, params)
}
