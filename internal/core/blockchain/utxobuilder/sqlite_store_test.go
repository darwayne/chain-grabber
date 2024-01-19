package utxobuilder

import (
	"context"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"testing"
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
