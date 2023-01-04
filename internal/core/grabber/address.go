package grabber

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

func PrivToPubKeyHash(key *btcec.PrivateKey, compressed bool, params *chaincfg.Params) (btcutil.Address, error) {
	var raw []byte
	if compressed {
		raw = key.PubKey().SerializeCompressed()
	} else {
		raw = key.PubKey().SerializeUncompressed()
	}
	pkHash := btcutil.Hash160(raw)
	return btcutil.NewAddressPubKeyHash(pkHash, params)
}

func PrivToPubKey(key *btcec.PrivateKey, compressed bool, params *chaincfg.Params) (btcutil.Address, error) {
	var raw []byte
	if compressed {
		raw = key.PubKey().SerializeCompressed()
	} else {
		raw = key.PubKey().SerializeUncompressed()
	}
	return btcutil.NewAddressPubKey(raw, params)
}

func PrivToSegwit(key *btcec.PrivateKey, compressed bool, params *chaincfg.Params) (btcutil.Address, error) {
	var raw []byte
	if compressed {
		raw = key.PubKey().SerializeCompressed()
	} else {
		raw = key.PubKey().SerializeUncompressed()
	}
	pubKeyHash := btcutil.Hash160(raw)
	return btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, params)
}

func PrivToTaprootPubKey(key *btcec.PrivateKey, params *chaincfg.Params) (btcutil.Address, error) {
	tapKey := txscript.ComputeTaprootKeyNoScript(key.PubKey())
	return btcutil.NewAddressTaproot(schnorr.SerializePubKey(tapKey), params)
}
