package keygen

import (
	"database/sql"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

func NewSQLReader(db *sql.DB, params *chaincfg.Params) *SQLReader {
	return &SQLReader{db: db, params: params}
}

type SQLReader struct {
	db     *sql.DB
	params *chaincfg.Params
}

func (r *SQLReader) Close() error {
	return r.db.Close()
}

func (r *SQLReader) HealthCheck() error {
	var val int
	return errors.Wrap(
		r.db.QueryRow("select key_id FROM addresses LIMIT 1").Scan(&val),
		"error checking db health")
}

func (r *SQLReader) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	row := r.db.QueryRow(`SELECT key_id, compressed FROM addresses WHERE address = ? LIMIT 1`, address.EncodeAddress())
	if err := row.Err(); err != nil {
		return nil, false, err
	}
	var keyID int
	var compressed int
	if err := row.Scan(&keyID, &compressed); err != nil {
		return nil, false, err
	}

	key := FromInt(keyID)

	return key, compressed == 1, nil
}

func (r *SQLReader) HasAddress(address btcutil.Address) (bool, error) {
	var exists int
	err := r.db.QueryRow(`SELECT (
    EXISTS(
    SELECT 1 FROM addresses WHERE address = ?)) AS row_exists`, address.EncodeAddress()).
		Scan(&exists)

	return exists == 1, err
}

func (r *SQLReader) GetScript(address btcutil.Address) ([]byte, error) {
	return txscript.PayToAddrScript(address)
}

func (r *SQLReader) ChainParams() *chaincfg.Params {
	return r.params
}

func (r *SQLReader) HasKey(key []byte) (bool, error) {
	var column = "compressed_pub_key"
	if len(key) > 33 {
		column = "pub_key"
	}
	var exists int
	err := r.db.QueryRow(`SELECT (
    EXISTS(
    	SELECT 1 FROM numbered_keys WHERE `+column+` = ? LIMIT 1
	)) AS row_exists`, key).
		Scan(&exists)

	return exists == 1, err
}

func (r *SQLReader) HasKnownCompressedKey(key []byte) (bool, error) {
	if len(key) != 33 {
		return false, nil
	}
	return r.HasKey(key)
}
