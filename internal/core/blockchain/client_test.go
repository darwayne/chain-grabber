package blockchain

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/mempool"
	"testing"
)

func TestClient(t *testing.T) {
	feeEstimator := mempool.NewFeeEstimator(
		mempool.DefaultEstimateFeeMaxRollback,
		mempool.DefaultEstimateFeeMinRegisteredBlocks)
	mempool.New(&mempool.Config{
		Policy: mempool.Policy{
			MaxTxVersion:         2,
			DisableRelayPriority: false,
			AcceptNonStd:         false,
			FreeTxRelayLimit:     15,
			MaxOrphanTxs:         100,
			MaxOrphanTxSize:      100000,
			MaxSigOpCostPerTx:    blockchain.MaxBlockSigOpsCost / 4,
			MinRelayTxFee:        mempool.DefaultMinRelayTxFee,
			RejectReplacement:    false,
		},
		ChainParams:  &chaincfg.MainNetParams,
		FeeEstimator: feeEstimator,
	})
}
