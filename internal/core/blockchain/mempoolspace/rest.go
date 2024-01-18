package mempoolspace

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/go-resty/resty/v2"
	"io"
	"net/http"
	"strconv"
)

type RestOpts struct {
	//::builder-gen -with-globals -prefix=With -no-builder
	HttpClient *http.Client
	Network    **chaincfg.Params
}
type Rest struct {
	cli *resty.Client
}

func NewRest(opts ...RestOptsFunc) *Rest {
	options := ToRestOpts(opts...)
	cli := resty.New()
	if options.HasHttpClient() {
		cli = resty.NewWithClient(options.HttpClient)
	}

	base := "https://mempool.space"
	if options.HasNetwork() {
		switch *options.Network {
		case &chaincfg.TestNet3Params:
			base += "/testnet"
		default:
			base += "/api"
		}
	} else {
		base += "/api"
	}

	cli.SetBaseURL(base)
	return &Rest{cli: cli}
}

func (r *Rest) GetBlockHeight(ctx context.Context) (int, error) {
	result, err := r.cli.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get("/blocks/tip/height")

	if err != nil {
		return 0, err
	}

	if result.StatusCode() != 200 {
		return 0, errors.New(fmt.Sprintf("unexpected status code: %d", result.StatusCode()))
	}

	rawNum, err := io.ReadAll(result.RawBody())
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(rawNum))
}

func (r *Rest) GetBlock(ctx context.Context, hash chainhash.Hash) (*wire.MsgBlock, error) {
	result, err := r.cli.R().
		SetContext(ctx).
		SetPathParam("hash", hash.String()).
		SetDoNotParseResponse(true).
		Get("/block/{hash}/raw")

	if err != nil {
		return nil, err
	}
	if result.StatusCode() != 200 {
		return nil, errors.New(fmt.Sprintf("unexpected status code: %d", result.StatusCode()))
	}

	block := &wire.MsgBlock{}
	body := result.RawBody()
	defer body.Close()
	if err := block.Deserialize(body); err != nil {
		return nil, err
	}

	return block, nil
}

func (r *Rest) GetBlockHashFromHeight(ctx context.Context, height int) (*chainhash.Hash, error) {
	result, err := r.cli.R().
		SetContext(ctx).
		SetPathParam("height", strconv.Itoa(height)).
		SetDoNotParseResponse(true).
		Get("/block-height/{height}")

	if err != nil {
		return nil, err
	}

	if result.StatusCode() != 200 {
		return nil, errors.New(fmt.Sprintf("unexpected status code: %d", result.StatusCode()))
	}

	body := result.RawBody()
	var buf bytes.Buffer
	if _, err = io.Copy(&buf, body); err != nil {
		return nil, err
	}
	defer body.Close()

	return chainhash.NewHashFromStr(buf.String())
}

func (r *Rest) GetTransaction(ctx context.Context, hash chainhash.Hash) (*wire.MsgTx, error) {
	result, err := r.cli.R().
		SetContext(ctx).
		SetPathParam("hash", hash.String()).
		SetDoNotParseResponse(true).
		Get("/tx/{hash}/hex")

	if err != nil {
		return nil, err
	}

	if result.StatusCode() != 200 {
		return nil, errors.New(fmt.Sprintf("unexpected status code: %d", result.StatusCode()))
	}

	block := &wire.MsgTx{}
	body := result.RawBody()
	reader := hex.NewDecoder(body)
	defer body.Close()
	if err := block.Deserialize(reader); err != nil {
		return nil, err
	}

	return block, nil
}

func (r *Rest) GetMempoolTransactionIDs(ctx context.Context) ([]chainhash.Hash, error) {
	var data []chainhash.Hash
	result, err := r.cli.R().
		SetContext(ctx).
		SetResult(&data).
		Get("/mempool/txids")

	if err != nil {
		return nil, err
	}

	if result.StatusCode() != 200 {
		return nil, errors.New(fmt.Sprintf("unexpected status code: %d", result.StatusCode()))
	}

	return data, nil
}

func (r *Rest) GetIPAddress(ctx context.Context) (string, error) {
	result := make(map[string]string)
	_, err := r.cli.R().
		SetContext(ctx).
		SetResult(&result).
		Get("https://api.ipify.org?format=json")
	if err != nil {
		return "", err
	}

	return result["ip"], nil
}
