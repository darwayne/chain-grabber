package noderpc

import (
	"context"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/blockchainmodels"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
)

type Client struct {
	cli *rpcclient.Client
}

func NewClient(host, user, pass string) (*Client, error) {
	connCfg := &rpcclient.ConnConfig{
		HTTPPostMode: true,
		DisableTLS:   true, // Bitcoin Core does not support HTTPS for RPC by default
		Host:         host,
		User:         user,
		Pass:         pass,
	}

	// Connect to RPC server
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}

	return &Client{cli: client}, nil
}

func (c *Client) GetAddressUTXOs(ctx context.Context, address string) ([]blockchainmodels.UTXO, error) {
	return nil, nil
}

func (c *Client) GetUTXO(ctx context.Context, outpoint wire.OutPoint) (*blockchainmodels.UTXO, error) {
	result, err := c.cli.GetTxOut(&outpoint.Hash, outpoint.Index, false)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	sats, _ := decimal.NewFromFloat(result.Value).Mul(
		decimal.NewFromFloat(100_000_000)).Float64()
	return &blockchainmodels.UTXO{
		Txid:  outpoint.Hash,
		Index: outpoint.Index,
		Value: int64(sats),
		Status: blockchainmodels.UTXOStatus{
			Confirmed: result.Confirmations > 0,
			BlockHash: result.BestBlock},
	}, nil
}

func (c *Client) GetTransaction(ctx context.Context, hash chainhash.Hash) (*wire.MsgTx, error) {
	result := c.cli.GetRawTransactionAsync(&hash)

	select {
	case res := <-result:
		result <- res
		info, err := result.Receive()
		if err != nil {
			return nil, err
		}

		return info.MsgTx(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) GetFee(ctx context.Context) (*blockchainmodels.Fee, error) {
	group, ctx := errgroup.WithContext(ctx)
	var minFee float64
	var fastFee float64
	group.Go(func() error {
		var err error
		minFee, err = c.getMinFee(ctx)

		return err
	})
	group.Go(func() error {
		var err error
		fastFee, err = c.getFee(ctx, 1)

		return err
	})

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return &blockchainmodels.Fee{
		Fastest: fastFee,
		Minimum: minFee,
	}, nil
}

func (c *Client) getFee(ctx context.Context, blocks int) (float64, error) {
	result := c.cli.EstimateSmartFeeAsync(int64(blocks), nil)

	select {
	case res := <-result:
		result <- res
		info, err := result.Receive()
		if err != nil {
			return 0, err
		}
		if len(info.Errors) > 0 {
			var err error
			for _, e := range info.Errors {
				err = multierr.Append(err, errors.New(e))
			}

			return 0, err
		}

		fee, _ := decimal.NewFromFloat(*info.FeeRate).Mul(
			decimal.NewFromFloat(100_000_000)).Div(
			decimal.NewFromFloat(1_000)).Float64()

		return fee, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (c *Client) getMinFee(ctx context.Context) (float64, error) {
	result := c.cli.GetNetworkInfoAsync()

	select {
	case res := <-result:
		result <- res
		info, err := result.Receive()
		if err != nil {
			return 0, err
		}

		fee, _ := decimal.NewFromFloat(info.RelayFee).Mul(
			decimal.NewFromFloat(100_000_000)).Div(
			decimal.NewFromFloat(1_000)).Float64()

		return fee, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
