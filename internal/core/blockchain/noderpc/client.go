package noderpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/blockchainmodels"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
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

type testMempoolAcceptCmd struct {
	RawTransactions []string `json:"rawtxs"`
}

func init() {
	btcjson.MustRegisterCmd("testmempoolaccept", (*testMempoolAcceptCmd)(nil), 0)
}

func (c *Client) TestMemPoolAccept(ctx context.Context, tx *wire.MsgTx) (*blockchainmodels.AcceptResult, error) {

	result := c.cli.SendCmd(&testMempoolAcceptCmd{RawTransactions: []string{
		txhelper.ToString(tx)},
	})

	type Stream struct {
		Data []blockchainmodels.AcceptResult
		Err  error
	}

	channel := make(chan Stream, 1)

	go func() {
		data, err := rpcclient.ReceiveFuture(result)
		if err != nil {
			channel <- Stream{Err: err}
			return
		}

		var r []blockchainmodels.AcceptResult
		if err := json.Unmarshal(data, &r); err != nil {
			channel <- Stream{Err: err}
			return
		}

		channel <- Stream{Data: r}
	}()
	select {
	case info := <-channel:
		if info.Err != nil {
			return nil, info.Err
		}

		return &info.Data[0], nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c Client) GetMemPoolEntry(ctx context.Context, hash chainhash.Hash) (*btcjson.GetMempoolEntryResult, error) {
	result := c.cli.GetMempoolEntryAsync(hash.String())

	select {
	case res := <-result:
		result <- res
		info, err := result.Receive()
		if err != nil {
			var jsonErr *btcjson.RPCError
			if errors.As(err, &jsonErr) {
				if jsonErr.Code == -5 {
					return nil, nil
				}
			}
			return nil, err
		}

		return info, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) GetUTXO(ctx context.Context, outpoint wire.OutPoint) (*blockchainmodels.UTXO, error) {
	result, err := c.cli.GetTxOut(&outpoint.Hash, outpoint.Index, true)
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
	result := c.cli.GetRawTransactionVerboseAsync(&hash)

	select {
	case res := <-result:
		result <- res
		info, err := result.Receive()
		if err != nil {
			return nil, err
		}

		return txhelper.FromString(info.Hex), nil
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

//
//func init() {
//	func() {
//		defer func() {
//			if r := recover(); r != nil {
//				fmt.Println("recovered!", r)
//			} else {
//				fmt.Println("registered??")
//			}
//		}()
//		btcjson.MustRegisterCmd("getmempoolinfo", (*btcjson.GetMempoolInfoCmd)(nil), 0)
//	}()
//
//}

func (c *Client) getMinFee(ctx context.Context) (float64, error) {
	result := c.cli.SendCmd(&btcjson.GetMempoolInfoCmd{})

	type MempoolResult struct {
		MinFee decimal.Decimal `json:"mempoolminfee"`
	}

	type Stream struct {
		Data MempoolResult
		Err  error
	}

	channel := make(chan Stream, 1)

	go func() {
		data, err := rpcclient.ReceiveFuture(result)
		if err != nil {
			channel <- Stream{Err: err}
			return
		}

		var r MempoolResult
		if err := json.Unmarshal(data, &r); err != nil {
			channel <- Stream{Err: err}
			return
		}

		channel <- Stream{Data: r}
	}()

	//result := c.cli.GetNetworkInfoAsync()

	select {
	case info := <-channel:
		if info.Err != nil {
			return 0, info.Err
		}

		fmt.Println(info.Data.MinFee.String())

		fee, _ := info.Data.MinFee.Mul(
			decimal.NewFromFloat(100_000_000)).Div(
			decimal.NewFromFloat(1_000)).Float64()

		return fee, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
