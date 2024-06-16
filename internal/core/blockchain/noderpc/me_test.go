package noderpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"
)

func TestMemFee(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	info := mempoolspace.NewRest()
	t.Log(info.GetFee(context.Background()))
}

func TestRealClient(t *testing.T) {
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")

	cli, err := NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)

	t.Log(cli.GetFee(context.Background()))

	var index uint32 = 1
	hash, err := chainhash.NewHashFromStr("c6212601f9f8c486c824864477a68ab9d9db989c129a268c842c7dd111a10983")
	require.NoError(t, err)

	result, err := cli.cli.GetTxOut(hash, index, false)

	t.Log(result, err)
}

func TestClient(t *testing.T) {

	// Connect to local bitcoin core RPC server using HTTP POST mode.
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST") // Replace with your bitcoind RPC server address

	// Connect to RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		HTTPPostMode: true,
		DisableTLS:   true, // Bitcoin Core does not support HTTPS for RPC by default
		Host:         rpcHost,
		User:         rpcUser,
		Pass:         rpcPass,
	}

	// Connect to RPC server
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatalf("Error creating new BTC client: %v", err)
	}
	defer client.Shutdown()

	hash, err := chainhash.NewHashFromStr("83ff2b04fe5e19f2650c5fedc706a26ab314e9edc40aed106373adaa36f6bf12")
	require.NoError(t, err)

	result, err := client.GetRawTransaction(hash)
	require.NoError(t, err)

	encoded, err := json.MarshalIndent(result.MsgTx(), "", " ")
	require.NoError(t, err)
	t.Log(string(encoded))

	// Example: Get blockchain info
	//info, err := client.GetBlockChainInfo()
	//if err != nil {
	//	log.Fatalf("Error getting blockchain info: %v", err)
	//}
	//
	//fmt.Printf("Best block hash: %s\n", info.BestBlockHash)
	//fmt.Printf("Blocks: %d\n", info.Blocks)
	//fmt.Printf("Difficulty: %f\n", info.Difficulty)

}

func TestMe(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	resp, err := client.Get("https://10.0.0.20:28332/") // Replace with your RPC endpoint
	if err != nil {
		log.Fatalf("Error making HTTPS request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}

	fmt.Printf("Response Body: %s\n", body)
}
