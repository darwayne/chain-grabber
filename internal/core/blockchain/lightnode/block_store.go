package lightnode

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sync/errgroup"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
)

type BlockStore struct {
	chain *chaincfg.Params
	db    *sql.DB
}

func NewBlockStore(chain *chaincfg.Params) *BlockStore {
	location := "./storedblocks/mainnet/blocks.sqlite"
	if chain != &chaincfg.MainNetParams {
		location = "./storedblocks/testnet/blocks.sqlite"
	}

	db, err := sql.Open("sqlite3", location) //+"?journal_mode=WAL")
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)
	setupDB(db)
	_, err = db.Exec("PRAGMA journal_mode = wal;")
	if err != nil {
		log.Fatal(err)
	}

	return &BlockStore{chain: chain, db: db}
}

func (b *BlockStore) DeleteBlock(ctx context.Context, hash chainhash.Hash) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM blocks WHERE hash = ?`, hash.String())
	return err
}

func (b *BlockStore) GetBlock(ctx context.Context, hash chainhash.Hash) (*wire.MsgBlock, error) {
	row := b.db.QueryRowContext(ctx, `select data from blocks WHERE hash = ?`, hash.String())
	if err := row.Err(); err != nil {
		return nil, err
	}
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, err
	}

	return bytesToBlock(data)

}
func (b *BlockStore) PutBlock(ctx context.Context, block *wire.MsgBlock) error {
	return b.PutBlocks(ctx, block)
}

type RawBlockStream struct {
	Hash string
	Data []byte
	Err  error
}

func (b *BlockStore) StreamRawBlock(ctx context.Context, after string) <-chan RawBlockStream {
	result := make(chan RawBlockStream, 1)
	go func() (e error) {
		defer func() {
			if e != nil {
				select {
				case result <- RawBlockStream{Err: e}:
				case <-ctx.Done():
					return
				}
			}

			close(result)
		}()

		var rows *sql.Rows
		var err error
		if after == "" {
			rows, err = b.db.QueryContext(ctx, `
select hash, data from blocks
ORDER BY hash asc
`)
		} else {
			rows, err = b.db.QueryContext(ctx, `
select hash, data from blocks
                  WHERE hash > ?
ORDER BY hash asc
`, after)
		}

		if err != nil {
			return err
		}
		for rows.Next() {
			var hash string
			var data []byte
			err = rows.Scan(&hash, &data)
			if err != nil {
				return err
			}

			select {
			case result <- RawBlockStream{Hash: hash, Data: data}:
			case <-ctx.Done():
				return
			}
		}

		return nil
	}()
	return result
}

func (b *BlockStore) PutBlocks(ctx context.Context, blocks ...*wire.MsgBlock) (e error) {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if e != nil {
			fmt.Println("an error detecting putting blocks .. rolling back tx", e)
			tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE into blocks VALUES(?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	type write struct {
		hash string
		data []byte
	}
	var mu sync.RWMutex
	var rowsToWrite []write
	var group errgroup.Group
	group.SetLimit(runtime.NumCPU())
	for _, block := range blocks {
		block := block
		group.Go(func() error {
			f := new(bytes.Buffer)
			writer, err := gzip.NewWriterLevel(f, gzip.BestCompression)
			if err != nil {
				return err
			}
			//writer := gzip.NewWriter(f)

			if err := block.Serialize(writer); err != nil {
				return err
			}
			if err := writer.Close(); err != nil {
				return err
			}

			mu.Lock()
			rowsToWrite = append(rowsToWrite, write{
				hash: block.BlockHash().String(),
				data: f.Bytes(),
			})
			mu.Unlock()

			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}

	mu.RLock()
	defer mu.RUnlock()
	for _, block := range rowsToWrite {
		_, err = stmt.ExecContext(
			ctx,
			block.hash, block.data)

		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (b *BlockStore) GetBlockFromHeight(ctx context.Context, height int) (*wire.MsgBlock, error) {
	row := b.db.QueryRowContext(ctx,
		`
select b.data from height h
            INNER JOIN blocks b on b.hash = h.hash
            WHERE height = ?`, height)
	if err := row.Err(); err != nil {
		return nil, err
	}
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, err
	}

	return bytesToBlock(data)
}

type BlockHeight struct {
	Height int
	Hash   chainhash.Hash
}

type BlockHeightStream struct {
	BlockHeight
	Err error
}

func (b *BlockStore) MissingBlocks(ctx context.Context) <-chan BlockHeightStream {
	result := make(chan BlockHeightStream, 1)

	go func() (e error) {
		defer func() {
			if e != nil {
				select {
				case result <- BlockHeightStream{Err: e}:
				case <-ctx.Done():
					return
				}
			}

			close(result)
		}()
		rows, err := b.db.QueryContext(ctx, `
select height, bh.hash, b.hash from height bh
LEFT JOIN blocks b ON b.hash = bh.hash
WHERE  b.hash IS  NULL
ORDER BY height asc
`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var height int
			var hash string
			var missingHash *string
			err = rows.Scan(&height, &hash, &missingHash)
			if err != nil {
				return err
			}

			h, err := chainhash.NewHashFromStr(hash)
			if err != nil {
				return err
			}
			data := BlockHeight{Height: height, Hash: *h}
			select {
			case result <- BlockHeightStream{BlockHeight: data}:
			case <-ctx.Done():
				return
			}
		}

		return nil
	}()

	return result
}

func (b *BlockStore) GetTip(ctx context.Context) (*BlockHeight, error) {
	row := b.db.QueryRowContext(ctx, `
select height, b.hash from height bh
LEFT JOIN blocks b ON b.hash = bh.hash
WHERE  b.hash IS NOT NULL
ORDER BY height desc
limit 1;
`)
	if err := row.Err(); err != nil {
		return nil, err
	}
	var height int
	var hash string
	err := row.Scan(&height, &hash)
	if err != nil {
		return nil, err
	}
	h, err := chainhash.NewHashFromStr(hash)
	if err != nil {
		return nil, err
	}
	return &BlockHeight{
		Height: height,
		Hash:   *h,
	}, nil
}

func (b *BlockStore) StreamHeight(ctx context.Context) <-chan BlockHeightStream {
	result := make(chan BlockHeightStream, 1)

	go func() (e error) {
		defer func() {
			if e != nil {
				select {
				case result <- BlockHeightStream{Err: e}:
				case <-ctx.Done():
					return
				}
			}

			close(result)
		}()
		rows, err := b.db.QueryContext(ctx, `
select height, hash from height 
ORDER BY height asc
`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var height int
			var hash string
			err = rows.Scan(&height, &hash)
			if err != nil {
				return err
			}

			h, err := chainhash.NewHashFromStr(hash)
			if err != nil {
				return err
			}
			data := BlockHeight{Height: height, Hash: *h}
			select {
			case result <- BlockHeightStream{BlockHeight: data}:
			case <-ctx.Done():
				return
			}
		}

		return nil
	}()

	return result
}

func (b *BlockStore) PutHeight(ctx context.Context, height BlockHeight) error {
	return b.PutHeights(ctx, height)
}

func (b *BlockStore) PutHeights(ctx context.Context, heights ...BlockHeight) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Commit()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE into height VALUES(?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	f := new(bytes.Buffer)
	for _, height := range heights {
		f.Reset()

		_, err = stmt.ExecContext(
			ctx,
			height.Height, height.Hash.String())

		if err != nil {
			return err
		}
	}

	return nil
}

func (b *BlockStore) Close() {
	b.db.Close()
}

func setupDB(db *sql.DB) {
	tables := []string{
		`
  CREATE TABLE IF NOT EXISTS height (
  height UNSIGNED BIG INT NOT NULL PRIMARY KEY,
  hash text NOT NULL
  );`,
		`
  CREATE TABLE IF NOT EXISTS blocks (
  hash text NOT NULL PRIMARY KEY,
  data BLOB NOT NULL
  );`,
	}

	for _, query := range tables {
		_, err := db.Exec(query)
		if err != nil {
			panic(err)
		}
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		select {
		case <-c:
		}
		fmt.Println("closing db")
		db.Close()
	}()

}

func bytesToBlock(data []byte) (*wire.MsgBlock, error) {
	buff := bytes.NewBuffer(data)
	reader, err := gzip.NewReader(buff)
	if err != nil {
		return nil, err
	}
	var block wire.MsgBlock
	if err := block.Deserialize(reader); err != nil {
		return nil, err
	}
	return &block, nil
}
