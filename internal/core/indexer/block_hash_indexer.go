package indexer

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/syndtr/goleveldb/leveldb"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type BlockHash struct {
	mu   sync.Mutex
	mngr *grabber.TransactionManager
}

func NewBlockHash(mngr *grabber.TransactionManager) BlockHash {
	return BlockHash{
		mngr: mngr,
	}
}

func (b *BlockHash) dbPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	initial := filepath.Join(home, "btc", "mainnet")
	hashDB := filepath.Join(initial, "blockhashdb")

	return hashDB, nil
}

func (b *BlockHash) getDB() (*leveldb.DB, error) {
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

func (b *BlockHash) HashFromHeight(height int64) (*chainhash.Hash, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	db, err := b.getDB()
	defer db.Close()

	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.LittleEndian, height); err != nil {
		return nil, err
	}

	result, err := db.Get(buf.Bytes(), nil)
	if err != nil {
		return nil, err
	}
	hash, err := chainhash.NewHash(result)
	if err != nil {
		return nil, err
	}

	return hash, nil
}

func (b *BlockHash) Index() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	dbPath, err := b.dbPath()
	if err != nil {
		return err
	}
	os.MkdirAll(dbPath, os.ModePerm)
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	mngr := b.mngr
	num, err := mngr.GetBlockCount()
	if err != nil {
		return err
	}

	var totalWorkDone int64
	var processed int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				v := atomic.LoadInt64(&processed)
				since := time.Since(start).Seconds()
				done := atomic.LoadInt64(&totalWorkDone)
				log.Printf("%.2f hashes saved per second (%d work done)[%d processed]", float64(done)/since, done, v)
			}
		}
	}()

	maxWorkers := 10
	errChan := make(chan error, maxWorkers)
	work := make(chan int64, maxWorkers)
	var wg sync.WaitGroup
	for x := 0; x < maxWorkers; x++ {
		go func() {
			buf := new(bytes.Buffer)
			for {
				select {
				case i := <-work:
					func() {
						defer wg.Done()
						buf.Reset()
						hash, err := mngr.GetBlockHash(i)
						if err != nil {
							errChan <- err
							return
						}

						err = binary.Write(buf, binary.LittleEndian, i)
						if err != nil {
							errChan <- err
							return
						}
						err = db.Put(buf.Bytes(), hash.CloneBytes(), nil)
						if err != nil {
							errChan <- err
							return
						}
						atomic.AddInt64(&processed, 1)
						atomic.AddInt64(&totalWorkDone, 1)

					}()

				}
			}
		}()
	}

	buf := new(bytes.Buffer)
	for i := num; i >= 0; i-- {
		buf.Reset()
		err = binary.Write(buf, binary.LittleEndian, i)
		if err != nil {
			return err
		}
		if _, err := db.Get(buf.Bytes(), nil); err != nil && err == leveldb.ErrNotFound {
			wg.Add(1)
			select {
			case work <- i:
			case err = <-errChan:
				return err
			}
			continue
		} else if err != nil {
			return err
		}
		atomic.AddInt64(&processed, 1)
	}

	wg.Wait()

	buf.Reset()
	if err = binary.Write(buf, binary.LittleEndian, num); err != nil {
		return err
	}
	return db.Put([]byte("max-idx"), buf.Bytes(), nil)
}

func (b *BlockHash) GetMaxCount() (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	db, err := b.getDB()
	if err != nil {
		return 0, nil
	}

	buf := new(bytes.Buffer)
	val, err := db.Get([]byte("max-idx"), nil)
	if err != nil {
		return 0, err
	}
	buf.Write(val)
	var result int64
	if err := binary.Read(buf, binary.LittleEndian, &result); err != nil {
		return 0, err
	}

	return result, nil

}

type BlockHashStream struct {
	Index int64
	Hash  chainhash.Hash
	Err   error
}

func (b *BlockHash) Iterate(ctx context.Context) chan BlockHashStream {
	results := make(chan BlockHashStream, 2)
	if err := b.Index(); err != nil {
		results <- BlockHashStream{Err: err}
		return results
	}

	db, err := b.getDB()
	defer db.Close()

	total, err := b.mngr.GetBlockCount()
	if err != nil {
		results <- BlockHashStream{Err: err}
		return results
	}

	go func() {
		buf := &bytes.Buffer{}
		for i := int64(0); i <= total; i++ {
			select {
			case <-ctx.Done():
				return
			default:

			}

			buf.Reset()

			if err := binary.Write(buf, binary.LittleEndian, i); err != nil {
				results <- BlockHashStream{Err: err}
				return
			}

			val, err := db.Get(buf.Bytes(), nil)
			if err != nil {
				results <- BlockHashStream{Err: err}
				return
			}
			hash, err := chainhash.NewHash(val)
			if err != nil {
				results <- BlockHashStream{Err: err}
				return
			}
			results <- BlockHashStream{
				Hash:  *hash,
				Index: i,
			}
		}
	}()

	return results
}
