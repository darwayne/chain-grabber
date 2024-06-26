package keygen

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"path"
)

type Reader struct {
	db     *leveldb.DB
	params *chaincfg.Params
}

func NewReaderFromDir(dir string, isTestNet bool) (*Reader, error) {
	f := dir
	params := &chaincfg.MainNetParams
	if isTestNet {
		f = path.Join(f, "testnet.db")
		params = &chaincfg.TestNet3Params
	} else {
		f = path.Join(f, "mainnet.db")
	}

	db, err := leveldb.OpenFile(f, &opt.Options{
		ErrorIfMissing: true,
		ReadOnly:       true,
	})
	if err != nil {
		return nil, err
	}

	return &Reader{
		db:     db,
		params: params,
	}, nil
}

func (r *Reader) Close() error {
	return r.db.Close()
}

func (r *Reader) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	raw, err := r.db.Get([]byte("address:"+address.EncodeAddress()), nil)
	if err != nil {
		return nil, false, errors.Wrap(err, "error getting from db")
	}

	wif, err := btcutil.DecodeWIF(string(raw))
	if err != nil {
		return nil, false, errors.Wrap(err, "error decoding wif")
	}

	return wif.PrivKey, wif.CompressPubKey, nil
}

func (r *Reader) HasAddress(address btcutil.Address) (bool, error) {
	return r.db.Has([]byte("address:"+address.EncodeAddress()), nil)
}

func (r *Reader) GetScript(address btcutil.Address) ([]byte, error) {
	return txscript.PayToAddrScript(address)
}

func (r *Reader) ChainParams() *chaincfg.Params {
	return r.params
}

func (r *Reader) HasKey(key []byte) (bool, error) {
	key = append([]byte("key:"), key...)

	found, err := r.db.Has(key, nil)

	return found, errors.Wrap(err, "error getting from db")
}

func (r *Reader) HasKnownCompressedKey(key []byte) (bool, error) {
	if len(key) != 33 {
		return false, nil
	}
	return r.HasKey(key)
}
