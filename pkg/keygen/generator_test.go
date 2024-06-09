package keygen

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func bulkWriterKeyOnly(t *testing.T, db *sql.DB, addresses [][]any, keys [][]any) {
	tx, err := db.Begin()
	require.NoError(t, err)
	keyStmt, err := tx.Prepare("INSERT OR IGNORE INTO numbered_keys VALUES (?, ?)")
	require.NoError(t, err)

	for _, keyBulk := range keys {
		_, err = keyStmt.Exec(keyBulk...)
		require.NoError(t, err)
	}

	defer keyStmt.Close()
	tx.Commit()
}

func bulkWriter(t *testing.T, db *sql.DB, addresses [][]any, keys [][]any) {
	tx, err := db.Begin()
	require.NoError(t, err)
	keyStmt, err := tx.Prepare("INSERT OR IGNORE INTO numbered_keys VALUES (?, ?, ?)")
	require.NoError(t, err)

	addrStmt, err := tx.Prepare("INSERT OR IGNORE INTO addresses VALUES (?, ?, ?)")
	require.NoError(t, err)

	for _, keyBulk := range keys {
		_, err = keyStmt.Exec(keyBulk...)
		require.NoError(t, err)
	}

	for _, bulk := range addresses {
		_, err = addrStmt.Exec(bulk...)
		require.NoError(t, err)
	}

	defer addrStmt.Close()
	defer keyStmt.Close()
	tx.Commit()
}

func bulkInserter(t *testing.T, db *sql.DB, wg *sync.WaitGroup, doneChan chan struct{}) (chan []any, chan []any) {
	keyChan := make(chan []any)
	addressChan := make(chan []any)

	t.Cleanup(func() {
		select {
		case <-doneChan:
		default:
			close(doneChan)
		}
	})

	go func() {
		endEarly := sigutil.Done()
		defer wg.Done()
		var keyArgs [][]any
		var addressArgs [][]any
		ticker := time.NewTicker(100 * time.Millisecond)
		_, _ = keyArgs, addressArgs

		for {
			select {
			case <-endEarly:
				return
			case keys := <-keyChan:
				_ = keys
				keyArgs = append(keyArgs, keys)
			case addresses := <-addressChan:
				_ = addresses
				addressArgs = append(addressArgs, addresses)
			case <-ticker.C:
				var isDone bool
				select {
				case <-doneChan:
					isDone = true
				default:
				}

				const arrSize = 200
				const batchSize = 10
				if !(len(keyArgs) > arrSize || len(addressArgs) > arrSize || isDone) {
					continue
				}
				log.Println("processing batch")

				keyChunks := ChunkSlice(keyArgs, batchSize)
				addrChunks := ChunkSlice(addressArgs, batchSize)

				keyMax := len(keyChunks)
				addrMax := len(addrChunks)

				largest := max(keyMax, addrMax)

				for i := 0; i < largest; i++ {
					var keys [][]any
					var addr [][]any

					if i < keyMax {
						keys = keyChunks[i]
					}
					if i < addrMax {
						addr = addrChunks[i]
					}

					bulkWriter(t, db, addr, keys)
				}

				addressArgs = nil
				keyArgs = nil

				log.Println("processing complete")

				if isDone {
					return
				}
			}
		}
	}()

	return keyChan, addressChan
}

func TestSqliteV2(t *testing.T) {
	db, err := sql.Open("sqlite3", "testdata/sampledbs/keydbv2.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	_, err = db.Exec("PRAGMA synchronous = OFF")
	require.NoError(t, err)

	_, err = db.Exec("PRAGMA journal_mode = MEMORY")
	require.NoError(t, err)

	const create string = `
  CREATE TABLE IF NOT EXISTS numbered_keys (
  id UNSIGNED BIG INT NOT NULL PRIMARY KEY,
  compressed_pub_key BLOB NOT NULL,
  pub_key BLOB NOT NULL
  );

CREATE INDEX IF NOT EXISTS compressed_pub_key_index ON numbered_keys(compressed_pub_key);
CREATE INDEX IF NOT EXISTS pub_key_index ON numbered_keys(pub_key);

  CREATE TABLE IF NOT EXISTS addresses (
  address text NOT NULL PRIMARY KEY,
  key_id UNSIGNED BIG INT NOT NULL,
  compressed INT NOT NULL default 0
  );

`

	_, err = db.Exec(create)
	require.NoError(t, err)

	var incr int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				processed := atomic.LoadInt64(&incr)
				t.Log("processed keys:", incr, "\tperformance:",
					fmt.Sprintf("%.2f", float64(processed)/time.Since(start).Seconds()))
			}
		}
	}()

	var wg sync.WaitGroup
	doneChan := make(chan struct{})
	wg.Add(1)
	endEarly := sigutil.Done()

	keyChan, addrChan := bulkInserter(t, db, &wg, doneChan)
loop:
	for info := range grabber.StreamKeys(1, 100_000_000) {
		require.NoError(t, info.Err)
		atomic.AddInt64(&incr, 1)
		pubCompressed := info.Key.PubKey().SerializeCompressed()
		pub := info.Key.PubKey().SerializeUncompressed()

		select {
		case <-endEarly:
			break loop
		case keyChan <- []any{info.Num, pubCompressed, pub}:
		}

		for _, addrInfo := range info.Addresses {
			var compressed int
			if addrInfo.Compressed {
				compressed = 1
			}

			select {
			case <-endEarly:
				break loop
			case addrChan <- []any{addrInfo.Address.EncodeAddress(), info.Num, compressed}:
			}

		}
	}

	select {
	case <-doneChan:
	default:
		close(doneChan)
	}

	wg.Wait()

}

func TestSqliteV3(t *testing.T) {
	db, err := sql.Open("sqlite3", "testdata/sampledbs/keydbv3.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	_, err = db.Exec("PRAGMA synchronous = OFF")
	require.NoError(t, err)

	const create string = `
  CREATE TABLE IF NOT EXISTS numbered_keys (
  id UNSIGNED BIG INT NOT NULL PRIMARY KEY,
  compressed_pub_key BLOB NOT NULL,
  pub_key BLOB NOT NULL
  );

CREATE INDEX IF NOT EXISTS compressed_pub_key_index ON numbered_keys(compressed_pub_key);
CREATE INDEX IF NOT EXISTS pub_key_index ON numbered_keys(pub_key);

  CREATE TABLE IF NOT EXISTS addresses (
  address text NOT NULL PRIMARY KEY,
  key_id UNSIGNED BIG INT NOT NULL,
  compressed INT NOT NULL default 0
  );

`

	_, err = db.Exec(create)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() {
		tx.Commit()
	})

	keyStmt, err := tx.Prepare("INSERT OR IGNORE INTO numbered_keys VALUES (?, ?, ?)")
	require.NoError(t, err)
	t.Cleanup(func() {
		keyStmt.Close()
	})

	addrStmt, err := tx.Prepare("INSERT OR IGNORE INTO addresses VALUES (?, ?, ?)")
	require.NoError(t, err)
	t.Cleanup(func() {
		addrStmt.Close()
	})

	var incr int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				processed := atomic.LoadInt64(&incr)
				t.Log("processed keys:", incr, "\tperformance:",
					fmt.Sprintf("%.2f", float64(processed)/time.Since(start).Seconds()))
			}
		}
	}()

	var wg sync.WaitGroup
	doneChan := make(chan struct{})
	wg.Add(1)
	for info := range grabber.StreamKeys(1, 100_000_000) {
		require.NoError(t, info.Err)
		atomic.AddInt64(&incr, 1)
		pubCompressed := info.Key.PubKey().SerializeCompressed()
		pub := info.Key.PubKey().SerializeUncompressed()
		_, err = keyStmt.Exec(info.Num, pubCompressed, pub)
		require.NoError(t, err)

		for _, addrInfo := range info.Addresses {
			var compressed int
			if addrInfo.Compressed {
				compressed = 1
			}

			_, err = addrStmt.Exec(addrInfo.Address.EncodeAddress(), info.Num, compressed)
			require.NoError(t, err)
		}
	}

	close(doneChan)
	wg.Wait()

}

func TestSqlite(t *testing.T) {
	db, err := sql.Open("sqlite3", "testdata/sampledbs/keydb.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	_, err = db.Exec("PRAGMA synchronous = OFF")
	require.NoError(t, err)

	const create string = `
  CREATE TABLE IF NOT EXISTS numbered_keys (
  id UNSIGNED BIG INT NOT NULL PRIMARY KEY,
  compressed_pub_key BLOB NOT NULL,
  pub_key BLOB NOT NULL
  );

CREATE INDEX IF NOT EXISTS compressed_pub_key_index ON numbered_keys(compressed_pub_key);
CREATE INDEX IF NOT EXISTS pub_key_index ON numbered_keys(pub_key);

  CREATE TABLE IF NOT EXISTS addresses (
  address text NOT NULL PRIMARY KEY,
  key_id UNSIGNED BIG INT NOT NULL,
  compressed INT NOT NULL default 0
  );

`

	_, err = db.Exec(create)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() {
		tx.Commit()
	})

	keyStmt, err := tx.Prepare("INSERT OR IGNORE INTO numbered_keys VALUES (?, ?, ?)")
	require.NoError(t, err)
	t.Cleanup(func() {
		keyStmt.Close()
	})

	addrStmt, err := tx.Prepare("INSERT OR IGNORE INTO addresses VALUES (?, ?, ?)")
	require.NoError(t, err)
	t.Cleanup(func() {
		addrStmt.Close()
	})

	var incr int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				processed := atomic.LoadInt64(&incr)
				t.Log("processed keys:", incr, "\tperformance:",
					fmt.Sprintf("%.2f", float64(processed)/time.Since(start).Seconds()))
			}
		}
	}()

	var wg sync.WaitGroup
	doneChan := make(chan struct{})
	wg.Add(1)
	for info := range grabber.StreamKeys(1, 100_000_000) {
		atomic.AddInt64(&incr, 1)
		pubCompressed := info.Key.PubKey().SerializeCompressed()
		pub := info.Key.PubKey().SerializeUncompressed()
		_, err = keyStmt.Exec(info.Num, pubCompressed, pub)
		require.NoError(t, err)

		var addr btcutil.Address
		for _, compressed := range []bool{true, false} {
			for _, network := range []*chaincfg.Params{&chaincfg.MainNetParams, &chaincfg.TestNet3Params} {
				addr, err = grabber.PrivToPubKeyHash(info.Key, compressed, network)
				require.NoError(t, err)
				var num int
				if compressed {
					num = 1
				}
				_, err = addrStmt.Exec(addr.EncodeAddress(), info.Num, num)
				require.NoError(t, err, "pub key hash")

				addr, err = grabber.PrivToSegwit(info.Key, compressed, network)
				require.NoError(t, err)

				_, err = addrStmt.Exec(addr.EncodeAddress(), info.Num, num)
				require.NoError(t, err, "segwit")

				if compressed {
					addr, err = grabber.PrivToTaprootPubKey(info.Key, network)
					require.NoError(t, err)

					_, err = addrStmt.Exec(addr.EncodeAddress(), info.Num, num)
					require.NoError(t, err, "taproot")

				}
			}
		}
	}

	close(doneChan)
	wg.Wait()

}

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
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		//os.RemoveAll(dbName)
		//os.Remove(dbName)
	})

	start := 1
	increment := 100
	end := start + increment
	var written int
	for i := 0; i < 10; i++ {
		log.Println("generating key range", start, end)
		info, err := grabber.GenerateKeys(start, end, params)
		log.Println("generated key range", start, end)
		require.NoError(t, err)
		for k, v := range info.AddressKeyMap {
			key := fmt.Sprintf("address:%s", k)
			//t.Log("adding address as:", key)
			err := db.Put([]byte(key), []byte(v.String()), &opt.WriteOptions{
				Sync: true,
			})
			written++
			require.NoError(t, err)
		}
		for k := range info.KnownKeys {
			decoded, _ := hex.DecodeString(k)
			key := append([]byte("key:"), decoded...)
			//t.Log("adding key as:", key)
			err := db.Put(key, nil, &opt.WriteOptions{
				Sync: true,
			})
			written++
			require.NoError(t, err)
		}
		start = end
		end = start + increment
	}

	t.Log("validating writes")

	require.NoError(t, db.Close())
	db, err = leveldb.OpenFile(dbName, nil)
	require.NoError(t, err)

	seen := 0
	info := db.NewIterator(&util.Range{}, &opt.ReadOptions{})
	for info.Next() {
		seen++
	}
	info.Release()
	//require.Equal(t, seen, written)
	t.Log(seen)
	_ = params

}

func TestChunkSlicer(t *testing.T) {
	myArr := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	t.Log(ChunkSlice(myArr, 2))
}

func ChunkSlice[T any](items []T, chunkSize int) (chunks [][]T) {
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}
	return append(chunks, items)
}

func BenchmarkKeyGenSpeed1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for range grabber.StreamKeysOnly(1, 100) {

		}
	}
}

func BenchmarkKeyGenSpeed2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for range grabber.StreamKeysOnlyOrdered(1, 100) {

		}
	}
}

func TestKeyOnlyDb(t *testing.T) {
	path := "testdata/sampledbs/keys_only.sqlite"
	//os.Remove(path)
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	_, err = db.Exec("PRAGMA synchronous = OFF")
	require.NoError(t, err)

	db.Exec(" PRAGMA page_size = 512")

	const create string = `
  CREATE TABLE IF NOT EXISTS numbered_keys (
	key BLOB NOT NULL  PRIMARY KEY,
	num UNSIGNED BIG INT NOT NULL
  );
`

	_, err = db.Exec(create)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() {
		tx.Commit()
	})

	keyStmt, err := tx.Prepare("INSERT OR IGNORE INTO numbered_keys VALUES (?, ?)")
	require.NoError(t, err)
	t.Cleanup(func() {
		keyStmt.Close()
	})

	var incr int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				processed := atomic.LoadInt64(&incr)
				t.Log("processed keys:", incr, "\tperformance:",
					fmt.Sprintf("%.2f", float64(processed)/time.Since(start).Seconds()))
			}
		}
	}()

	for info := range grabber.StreamKeysOnlyOrdered(1, 100_000_000) {
		keyStmt.Exec(info.Key.PubKey().SerializeCompressed(), info.Num)
		num := atomic.AddInt64(&incr, 1)
		if num%90_000 == 0 {
			tx.Commit()
			keyStmt.Close()
			tx, err = db.Begin()
			require.NoError(t, err)

			keyStmt, err = tx.Prepare("INSERT OR IGNORE INTO numbered_keys VALUES (?, ?)")
			require.NoError(t, err)
		}
	}

}

func TestKeyOnlyDbOptimized(t *testing.T) {
	path := "testdata/sampledbs/keys_only_100-200b.sqlite"
	//os.Remove(path)
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	_, err = db.Exec("PRAGMA synchronous = OFF")
	require.NoError(t, err)

	db.Exec(" PRAGMA page_size = 512")

	const create string = `
  CREATE TABLE IF NOT EXISTS numbered_keys_%03d (
	key BLOB NOT NULL  PRIMARY KEY,
	num UNSIGNED BIG INT NOT NULL
  );
`

	r := rand.New(rand.NewSource(1))

	const batchSize = 5
	var builder strings.Builder
	for i := 1; i <= batchSize; i++ {
		builder.WriteString(fmt.Sprintf(create, i))
	}

	_, err = db.Exec(builder.String())
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() {
		tx.Commit()
	})

	var keyStmts []*sql.Stmt
	var genKeyStatements func()
	genKeyStatements = func() {
		if len(keyStmts) > 0 {
			tx.Commit()
			for _, k := range keyStmts {
				k.Close()
			}

			tx, err = db.Begin()
			require.NoError(t, err)
			keyStmts = nil
			genKeyStatements()
			return
		}
		for i := 1; i <= batchSize; i++ {
			keyStmt, err := tx.Prepare(fmt.Sprintf("INSERT OR IGNORE INTO numbered_keys_%03d VALUES (?, ?)", i))
			require.NoError(t, err)
			keyStmts = append(keyStmts, keyStmt)
		}
	}

	genKeyStatements()

	var incr int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				processed := atomic.LoadInt64(&incr)
				t.Log("processed keys:", incr, "\tperformance:",
					fmt.Sprintf("%.2f", float64(processed)/time.Since(start).Seconds()))
			}
		}
	}()

	var write func([]byte, int)
	write = func(key []byte, keyNum int) {
		idx := int(r.Int31n(batchSize))
		keyStmts[idx].Exec(key, keyNum)
		atomic.AddInt64(&incr, 1)
	}

	var flush func()
	flush = func() {
		genKeyStatements()
	}

	var lastKey int
	t.Cleanup(func() {
		t.Log("last key was", lastKey)
	})
	for info := range grabber.StreamKeysOnlyOrdered(100_000_000, 200_000_000) {
		key := info.Key.PubKey().SerializeCompressed()
		write(key, info.Num)
		if atomic.LoadInt64(&incr)%180_000 == 0 {
			flush()
		}

		lastKey = info.Num
	}

	flush()
}

func TestTotalKeysGenerated(t *testing.T) {
	start := time.Now()
	compressedMap := make(map[[33]byte]uint32)
	for stream := range grabber.StreamKeysOnly(1, 1_000) {
		_, _ = stream, compressedMap

		t.Log(stream.Num)
		//key := stream.Key.PubKey()
		//compressedMap[[33]byte(key.SerializeCompressed())] = uint32(stream.Num)
	}

	fmt.Println("completed in", time.Since(start))

	// bytes as int:   53968372
	// bytes as uin32: 53935412
	//var buff bytes.Buffer
	//enc := gob.NewEncoder(&buff)
	//err := enc.Encode(compressedMap)
	//require.NoError(t, err)
	//fmt.Println(buff.Len(), "bytes")

	sigutil.Wait()

	//select {
	//case <-sigutil.Done():
	//default:
	//
	//}

}
