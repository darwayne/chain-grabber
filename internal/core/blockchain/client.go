package blockchain

import (
	"context"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type Client interface {
	GetBlock(ctx context.Context, hash chainhash.Hash) (*wire.MsgBlock, error)
	GetBlockHashFromHeight(ctx context.Context, height int) (*chainhash.Hash, error)
	GetBlockHeight(ctx context.Context) (int, error)
}
