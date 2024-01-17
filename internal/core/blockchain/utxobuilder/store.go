package utxobuilder

import (
	"compress/gzip"
	"context"
	"encoding/gob"
	"errors"
	"github.com/btcsuite/btcd/wire"
	"os"
)

func NewUTXOStore(filepath string) UTXOStore {
	return UTXOStore{filepath: filepath}
}

type UTXOStore struct {
	filepath string
}

func (u UTXOStore) Put(_ context.Context, set UTXOSet) error {
	file, err := os.CreateTemp(os.TempDir(), "btc-utxoset")
	if err != nil {
		return err
	}

	writer, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	encoder := gob.NewEncoder(writer)
	if err := encoder.Encode(set); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), u.filepath); err != nil {
		return err
	}

	return nil

}

func (u UTXOStore) Get(_ context.Context) (UTXOSet, error) {
	file, err := os.Open(u.filepath)
	if errors.Is(err, os.ErrNotExist) {
		return UTXOSet{
			UTXOs: make(map[wire.OutPoint]int64),
		}, nil
	}
	if err != nil {
		return UTXOSet{}, err
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return UTXOSet{}, err
	}
	defer reader.Close()

	var result UTXOSet
	if err := gob.NewDecoder(reader).Decode(&result); err != nil {
		return UTXOSet{}, err
	}

	return result, nil
}
