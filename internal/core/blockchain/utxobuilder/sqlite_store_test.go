package utxobuilder

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSQLite(t *testing.T) {
	db, err := sql.Open("sqlite3", "mainnet.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})
	const create string = `
  CREATE TABLE IF NOT EXISTS utxos (
  id text NOT NULL PRIMARY KEY,
  value UNSIGNED BIG INT NOT NULL
  );`

	_, err = db.Exec(create)
	require.NoError(t, err)

	store := NewUTXOStore("./testdata/backup.mainnet.gob.gz")
	data, err := store.Get(context.TODO())
	require.NoError(t, err)

	// TOOK: 35936
	for k, v := range data.UTXOs {
		_, err = db.Exec(`INSERT INTO utxos VALUES(?, ?);`, k.String(), v)
		require.NoError(t, err)
	}

}

func TestSQLiteBlockHeight(t *testing.T) {
	db, err := sql.Open("sqlite3", "main-net-block-height.sqlite")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})
	const create string = `
  CREATE TABLE IF NOT EXISTS block_height (
  height UNSIGNED BIG INT NOT NULL PRIMARY KEY,
  hash text NOT NULL
  );`

	_, err = db.Exec(create)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		for range c {
			cancel()
			return
		}
	}()

	start := time.Now()
	var rows int64

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			time.Sleep(time.Second)
			rate := float64(atomic.LoadInt64(&rows)) / float64(time.Since(start).Seconds())
			t.Logf("%.2f rows inserted per second\n", rate)
		}
	}()

	var arr []int
	err = filepath.WalkDir("./testdata/blockchairblockdata", func(path string, d fs.DirEntry, err error) (e error) {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		defer func() {
			if e != nil {
				e = errors.Wrap(e, path)
			}
		}()
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".gz") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		reader, err := gzip.NewReader(f)
		if err != nil {
			fmt.Println("invalid data in", path)
			os.Remove(path)
			return nil
		}
		defer reader.Close()
		r := csv.NewReader(reader)
		r.Comma = '\t'
		r.ReuseRecord = true
		r.Read()
		//i := 0
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			default:
			}
			record, err := r.Read()
			if err == io.EOF {
				return nil
			}

			height, err := strconv.Atoi(record[0])
			arr = append(arr, height)
			require.NoError(t, err)
			_ = height
			//fmt.Println("height:", height, "\t", "hash:", record[1])
			_, err = db.Exec(`INSERT OR IGNORE INTO block_height VALUES(?, ?);`, height, record[1])
			atomic.AddInt64(&rows, 1)
			require.NoError(t, err)
		}

		return nil
	})

	require.NoError(t, err)

	var totalMissing int
	sort.Ints(arr)
	for i := 1; i < len(arr); i++ {
		if arr[i]-arr[i-1] != 1 {
			totalMissing += arr[i] - arr[i-1]
			t.Log("missing blocks between", arr[i-1], "and", arr[i])
		}
	}

	t.Log(totalMissing, "total missing blocks")
	t.Log(len(arr), "blocks")

}
