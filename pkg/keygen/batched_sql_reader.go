package keygen

import (
	"database/sql"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/pkg/errors"
)

const batchSize = 5

func NewBatchedSQLReader(db *sql.DB, params *chaincfg.Params) *BatchedSQLReader {
	return &BatchedSQLReader{db: db, params: params}
}

type BatchedSQLReader struct {
	db     *sql.DB
	params *chaincfg.Params
}

func (r *BatchedSQLReader) Close() error {
	return r.db.Close()
}

func (r *BatchedSQLReader) HealthCheck() error {
	var val int
	return errors.Wrap(
		r.db.QueryRow("select key_id FROM addresses LIMIT 1").Scan(&val),
		"error checking db health")
}

func (r *BatchedSQLReader) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
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

func (r *BatchedSQLReader) HasAddress(address btcutil.Address) (bool, error) {
	var exists int
	err := r.db.QueryRow(`SELECT (
    EXISTS(
    SELECT 1 FROM addresses WHERE address = ?)) AS row_exists`, address.EncodeAddress()).
		Scan(&exists)

	return exists == 1, err
}

func (r *BatchedSQLReader) GetScript(address btcutil.Address) ([]byte, error) {
	return txscript.PayToAddrScript(address)
}

func (r *BatchedSQLReader) ChainParams() *chaincfg.Params {
	return r.params
}

func (r *BatchedSQLReader) HasKey(key []byte) (bool, error) {
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

func (r *BatchedSQLReader) HasKnownCompressedKey(key []byte) (bool, error) {
	if len(key) != 33 {
		return false, nil
	}
	return r.HasKey(key)
}
