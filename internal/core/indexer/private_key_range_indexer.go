package indexer

import (
	"bytes"
	"encoding/gob"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/syndtr/goleveldb/leveldb"
	"os"
	"path/filepath"
	"sync"
)

type PrivateKeyRangeIndexer struct {
}

func (b *PrivateKeyRangeIndexer) dbPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	initial := filepath.Join(home, "btc", "testnet")
	hashDB := filepath.Join(initial, "p_addr_index")

	return hashDB, nil
}

func (b *PrivateKeyRangeIndexer) getDB() (*leveldb.DB, error) {
	dbPath, err := b.dbPath()
	if err != nil {
		return nil, err
	}

	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, err
	}

	return db, nil
}

type PrivateKeyInfo struct {
	N          []byte
	Compressed bool
}

var privateKeyRangeIndexerMu sync.RWMutex

func (b PrivateKeyRangeIndexer) WifFromAddress(address string, param *chaincfg.Params) (*btcutil.WIF, error) {
	privateKeyRangeIndexerMu.Lock()
	defer privateKeyRangeIndexerMu.Unlock()
	db, err := b.getDB()
	if err != nil {
		return nil, err
	}

	value, err := db.Get([]byte(address), nil)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(value)
	decoder := gob.NewDecoder(buf)
	var info PrivateKeyInfo
	if err := decoder.Decode(&info); err != nil {
		return nil, err
	}
	key := secp256k1.PrivKeyFromBytes(info.N)
	wif, err := btcutil.NewWIF(key, param, info.Compressed)
	if err != nil {
		return nil, err
	}

	return wif, nil
}

func (b PrivateKeyRangeIndexer) Index(param *chaincfg.Params) error {
	privateKeyRangeIndexerMu.Lock()
	defer privateKeyRangeIndexerMu.Unlock()
	p, err := b.dbPath()
	if err != nil {
		return err
	}

	os.MkdirAll(p, os.ModePerm)
	keys, err := grabber.GenerateKeys(0, 50_000, param)
	if err != nil {
		return err
	}

	db, err := b.getDB()
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	for address, key := range keys.AddressKeyMap {
		buf.Reset()
		info := PrivateKeyInfo{
			N:          key.PrivKey.Serialize(),
			Compressed: key.CompressPubKey,
		}
		if err := enc.Encode(info); err != nil {
			return err
		}

		if err := db.Put([]byte(address), buf.Bytes(), nil); err != nil {
			return err
		}
	}

	return nil
}
