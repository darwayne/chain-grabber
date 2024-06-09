package keygen

import (
	"crypto/sha512"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

func BrainKey(password string, cfg *chaincfg.Params, derivationPath ...int) (*hdkeychain.ExtendedKey, error) {
	hasher := sha512.New()                          // <-- use sha512 since it generates the highest complexity key
	hasher.Write([]byte("chain-grabber-brain-key")) // <-- salt to avoid potential weak keys
	hasher.Write([]byte(password))
	key, err := hdkeychain.NewMaster(hasher.Sum(nil), cfg)
	if err != nil {
		return nil, err
	}
	for _, num := range derivationPath {
		key, err = key.Derive(uint32(num))
		if err != nil {
			return nil, err
		}
	}
	return key, err
}
