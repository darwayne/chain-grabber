package keygen

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/errutil"
)

type secretStore interface {
	txauthor.SecretsSource
	HasAddress(address btcutil.Address) (bool, error)
	HasKey(key []byte) (bool, error)
	HasKnownCompressedKey(key []byte) (bool, error)
}

var _ secretStore = (*MultiReader)(nil)

func NewMultiReader(readers ...secretStore) MultiReader {
	return MultiReader{readers: readers}
}

type MultiReader struct {
	readers []secretStore
}

func (m MultiReader) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	for _, r := range m.readers {
		a, b, err := r.GetKey(address)
		if err != nil {
			continue
		}

		return a, b, nil
	}
	return nil, false, errutil.NewNotFound("not found")
}

func (m MultiReader) GetScript(address btcutil.Address) ([]byte, error) {
	for _, r := range m.readers {
		a, err := r.GetScript(address)
		if err != nil {
			continue
		}

		return a, nil
	}
	return nil, errutil.NewNotFound("not found")
}

func (m MultiReader) ChainParams() *chaincfg.Params {
	for _, r := range m.readers {
		return r.ChainParams()
	}
	return nil
}

func (m MultiReader) HasAddress(address btcutil.Address) (bool, error) {
	for _, r := range m.readers {
		a, err := r.HasAddress(address)
		if err != nil || !a {
			continue
		}

		return a, nil
	}
	return false, nil
}

func (m MultiReader) HasKey(key []byte) (bool, error) {
	for _, r := range m.readers {
		a, err := r.HasKey(key)
		if err != nil || !a {
			continue
		}

		return a, nil
	}
	return false, nil
}

func (m MultiReader) HasKnownCompressedKey(key []byte) (bool, error) {
	for _, r := range m.readers {
		a, err := r.HasKnownCompressedKey(key)
		if err != nil || !a {
			continue
		}

		return a, nil
	}
	return false, nil
}
