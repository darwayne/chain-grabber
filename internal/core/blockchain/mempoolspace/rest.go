package mempoolspace

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/blockchainmodels"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type RestOpts struct {
	//::builder-gen -with-globals -prefix=With -no-builder
	HttpClient *http.Client
	Network    **chaincfg.Params
}

//var _ memspender.NetworkGrabber = (*Rest)(nil)

func NewRest(opts ...RestOptsFunc) *Rest {
	options := ToRestOpts(opts...)
	cli := resty.New()
	if options.HasHttpClient() {
		cli = resty.NewWithClient(options.HttpClient)
	} else if os.Getenv("PROXY_USER") != "" {

		d, err := proxy.SOCKS5("tcp", "phx.socks.ipvanish.com:1080", &proxy.Auth{
			User:     os.Getenv("PROXY_USER"),
			Password: os.Getenv("PROXY_PASS"),
		}, proxy.Direct)
		if err != nil {
			panic(err)
		}
		// Create a transport with the SOCKS5 proxy dialer
		transport := &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				rawConn, err := d.Dial(network, addr)
				if err != nil {
					return nil, err
				}
				cli := tls.Client(rawConn, &tls.Config{
					ServerName: strings.Split(addr, ":")[0],
				})
				if err := cli.HandshakeContext(ctx); err != nil {
					rawConn.Close()
					return nil, errors.Wrapf(err, "error creating handshake to: %s", addr)
				}

				return cli, nil
			},
		}

		// Create an HTTP client with the custom transport
		client := &http.Client{
			Transport: transport,
		}
		cli = resty.NewWithClient(client)
	}

	base := "https://mempool.space"
	if options.HasNetwork() {
		switch *options.Network {
		case &chaincfg.TestNet3Params:
			base += "/testnet/api"
		default:
			base += "/api"
		}
	} else {
		base += "/api"
	}

	waitForWork := make(chan struct{})
	go func() {
		duration := 500 * time.Millisecond
		for {
			select {
			case <-waitForWork:
				time.Sleep(duration)
			}
		}
	}()
	cli.OnBeforeRequest(func(_ *resty.Client, r *resty.Request) error {
		r.SetHeader("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		waitForWork <- struct{}{}
		return nil
	})

	cli.SetBaseURL(base)
	res := &Rest{cli: cli}
	//res.WithTrace().WithDebugging()
	return res
}

type Rest struct {
	cli *resty.Client
}

func (r *Rest) GetMemPoolEntry(ctx context.Context, hash chainhash.Hash) (*btcjson.GetMempoolEntryResult, error) {
	return nil, nil
}
func (r *Rest) TestMemPoolAccept(ctx context.Context, tx *wire.MsgTx) (*blockchainmodels.AcceptResult, error) {
	return &blockchainmodels.AcceptResult{Allowed: true}, nil
}

func (r *Rest) BroadCast(ctx context.Context, tx *wire.MsgTx) error {
	var buf bytes.Buffer
	tx.Serialize(&buf)

	return r.BroadcastHex(ctx, hex.EncodeToString(buf.Bytes()))
}

func (r *Rest) BroadcastHex(ctx context.Context, str string) error {
	result, err := r.cli.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetBody([]byte(str)).
		Post("/tx")

	if err != nil {
		return err
	}

	if result.StatusCode() != 200 {
		var body string
		rawBody := result.RawBody()
		defer rawBody.Close()
		data, err := io.ReadAll(result.RawBody())
		if err == nil {
			body = string(data)
		}

		return errors.New(fmt.Sprintf("unexpected status code: %d\nbody:%s", result.StatusCode(), body))
	}

	return nil
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

type Vout struct {
	Scriptpubkey        string `json:"scriptpubkey"`
	ScriptpubkeyAsm     string `json:"scriptpubkey_asm"`
	ScriptpubkeyType    string `json:"scriptpubkey_type"`
	ScriptpubkeyAddress string `json:"scriptpubkey_address"`
	Value               int64  `json:"value"`
}
type Transaction struct {
	Txid     chainhash.Hash `json:"txid"`
	Version  int            `json:"version"`
	Locktime int            `json:"locktime"`
	Vin      []struct {
		Txid    chainhash.Hash `json:"txid"`
		Vout    int            `json:"vout"`
		Prevout struct {
			Scriptpubkey        string `json:"scriptpubkey"`
			ScriptpubkeyAsm     string `json:"scriptpubkey_asm"`
			ScriptpubkeyType    string `json:"scriptpubkey_type"`
			ScriptpubkeyAddress string `json:"scriptpubkey_address"`
			Value               int    `json:"value"`
		} `json:"prevout"`
		Scriptsig    string   `json:"scriptsig"`
		ScriptsigAsm string   `json:"scriptsig_asm"`
		Witness      []string `json:"witness"`
		IsCoinbase   bool     `json:"is_coinbase"`
		Sequence     int64    `json:"sequence"`
	} `json:"vin"`
	Vout   []Vout `json:"vout"`
	Size   int    `json:"size"`
	Weight int    `json:"weight"`
	Sigops int    `json:"sigops"`
	Fee    int    `json:"fee"`
	Status struct {
		Confirmed   bool   `json:"confirmed"`
		BlockHeight int    `json:"block_height"`
		BlockHash   string `json:"block_hash"`
		BlockTime   int    `json:"block_time"`
	} `json:"status"`
}

func (r *Rest) GetAddressTransactions(ctx context.Context, address string, afterTxID ...chainhash.Hash) ([]Transaction, error) {
	params := make(map[string]string)
	if len(afterTxID) == 1 {
		params["after_txid"] = afterTxID[0].String()
	}
	var result []Transaction
	_, err := r.cli.R().
		SetContext(ctx).
		SetResult(&result).
		SetQueryParams(params).
		SetPathParam("address", address).
		Get("address/{address}/txs")
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (r *Rest) GetUTXO(ctx context.Context, outpoint wire.OutPoint) (*blockchainmodels.UTXO, error) {
	results, err := r.TransactionOutSpends(ctx, outpoint.Hash)
	if err != nil {
		return nil, err
	}

	var result *blockchainmodels.UTXO = nil

	for _, rr := range results {
		if uint32(rr.Vin) == outpoint.Index && rr.Txid.IsEqual(&outpoint.Hash) {
			tx, err := r.GetTransaction(ctx, outpoint.Hash)
			if err != nil {
				return nil, err
			}
			return &blockchainmodels.UTXO{
				Txid:   rr.Txid,
				Index:  uint32(rr.Vin),
				Status: blockchainmodels.UTXOStatus{},
				Value:  tx.TxOut[rr.Vin].Value,
			}, nil
		}
	}

	return result, nil
}

func (r *Rest) GetAddressUTXOs(ctx context.Context, address string) ([]blockchainmodels.UTXO, error) {
	var result []blockchainmodels.UTXO
	_, err := r.cli.R().
		SetContext(ctx).
		SetResult(&result).
		SetPathParam("address", address).
		Get("address/{address}/utxo")
	if err != nil {
		return nil, err
	}

	return result, nil
}

type TransactionOutSpend struct {
	Spent  bool                        `json:"spent"`
	Txid   chainhash.Hash              `json:"txid"`
	Vin    int                         `json:"vin"`
	Status blockchainmodels.UTXOStatus `json:"status"`
}

func (r *Rest) TransactionOutSpends(ctx context.Context, txID chainhash.Hash) ([]TransactionOutSpend, error) {
	var result []TransactionOutSpend
	_, err := r.cli.R().
		SetContext(ctx).
		SetResult(&result).
		SetPathParam("txId", txID.String()).
		Get("tx/{txId}/outspends")
	if err != nil {
		return nil, err
	}

	return result, nil
}

type Fee struct {
	Fastest float64 `json:"fastestFee"`
	Minimum float64 `json:"minimumFee"`
}

func (r *Rest) GetFee(ctx context.Context) (*blockchainmodels.Fee, error) {
	var result blockchainmodels.Fee
	_, err := r.cli.R().
		SetContext(ctx).
		SetResult(&result).
		Get("v1/fees/recommended")
	if err != nil {
		return nil, err
	}

	return &result, nil
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

func (r *Rest) WithTrace() *Rest {
	r.cli.EnableTrace()
	return r
}

func (r *Rest) WithDebugging() *Rest {
	r.cli.SetDebug(true)
	return r
}
