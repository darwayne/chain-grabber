package keygen

import "github.com/btcsuite/btcd/btcec/v2"

func FromInt(num int) *btcec.PrivateKey {
	var mod btcec.ModNScalar
	mod.Zero()
	mod.SetInt(uint32(num))
	return btcec.PrivKeyFromScalar(&mod)
}
