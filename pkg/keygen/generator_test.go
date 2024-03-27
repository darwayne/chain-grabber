package keygen

import (
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"log"
	"testing"
)

func TestLevelDB(t *testing.T) {
	var isTestNet bool
	dbName := "testdata/sampledbs/"
	params := &chaincfg.MainNetParams
	if isTestNet {
		dbName += "testnet.db"
		params = &chaincfg.TestNet3Params
	} else {
		dbName += "mainnet.db"
	}
	db, err := leveldb.OpenFile(dbName, nil)
	if err != nil {
		fmt.Println("Error opening database:", err)
		return
	}

	start := 1
	increment := 1_000_000
	end := start + increment
	for i := 0; i < 100; i++ {
		log.Println("generating key range", start, end)
		info, err := grabber.GenerateKeys(start, end, params)
		log.Println("generated key range", start, end)
		require.NoError(t, err)
		for k, v := range info.AddressKeyMap {
			db.Put([]byte(k), []byte(v.String()), nil)
		}
		for k, v := range info.KnownKeys {
			db.Put(k[:], []byte(v.String()), nil)
		}
		start = end
		end = start + increment
	}

	t.Cleanup(func() {
		db.Close()
		//os.RemoveAll(dbName)
		//os.Remove(dbName)
	})
}
