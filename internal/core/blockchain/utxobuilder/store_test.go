package utxobuilder

import (
	"context"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewUTXOStore(t *testing.T) {
	testPath := "./testdata/test.gz"

	h, err := chainhash.NewHashFromStr("141b7dfae5659f5c87bb650a6dba7659ba3dac6b8763767b2073351705ab649d")
	require.NoError(t, err)

	store := NewUTXOStore(testPath)
	err = store.Put(context.Background(), UTXOSet{
		StartPoint: 14,
		UTXOs: map[wire.OutPoint]int64{
			wire.OutPoint{
				Hash:  *h,
				Index: 1,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 2,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 3,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 4,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 5,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 6,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 7,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 8,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 9,
			}: 3,
			wire.OutPoint{
				Hash:  *h,
				Index: 10,
			}: 1337,
			wire.OutPoint{
				Hash:  *h,
				Index: 11,
			}: 11_000,
		},
	})
	require.NoError(t, err)

	data, err := store.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, data.UTXOs, 11)
	require.Equal(t, 14, data.StartPoint)
	require.Equal(t, int64(1337), data.UTXOs[wire.OutPoint{
		Hash:  *h,
		Index: 10,
	}])
}
