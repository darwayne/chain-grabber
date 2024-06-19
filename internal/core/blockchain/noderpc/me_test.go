package noderpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/fxamacker/cbor"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestBinaryCom(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	jBuf := bytes.NewBuffer(nil)
	mBuf := bytes.NewBuffer(nil)
	enc := cbor.NewEncoder(buf, cbor.CanonicalEncOptions())
	jEnc := json.NewEncoder(jBuf)
	mEnc := msgpack.NewEncoder(mBuf)

	type yo struct {
		Son bool
		Msg string
		Num int
		Any any
	}

	orig := yo{Son: true, Msg: "what up", Num: 1, Any: map[string]any{
		"hey": map[string]any{
			"there": 13.37,
		},
	}}
	start := time.Now()
	err := enc.Encode(orig)
	took := time.Since(start)
	//require.NoError(t, err)
	t.Log(buf.Len())

	t.Log("cbor encode", took, "data size", buf.Len())

	start = time.Now()
	jEnc.Encode(orig)
	took = time.Since(start)
	t.Log("json encode", took, "data size", jBuf.Len())

	start = time.Now()
	mEnc.Encode(orig)
	took = time.Since(start)
	t.Log("msgpack encode", took, "data size", mBuf.Len())

	start = time.Now()
	rawBuffer := bytes.NewBuffer(nil)
	err = binary.Write(rawBuffer, binary.BigEndian, int32(buf.Len()))
	//require.NoError(t, err)
	_, err = io.Copy(rawBuffer, buf)
	took = time.Since(start)
	//require.NoError(t, err)
	t.Log("writing cbor", took)

	start = time.Now()
	var length int32
	err = binary.Read(rawBuffer, binary.BigEndian, &length)
	//require.NoError(t, err)
	//t.Log(length)

	var hmm yo

	err = cbor.NewDecoder(io.LimitReader(rawBuffer, int64(length))).Decode(&hmm)
	took = time.Since(start)
	require.NoError(t, err)

	t.Log("cbor decode", took)

	t.Logf("hmm: %+v", hmm)
	t.Logf("orig: %+v", orig)

	//=== json starts here
	start = time.Now()
	rawBuffer2 := bytes.NewBuffer(nil)
	err = binary.Write(rawBuffer2, binary.BigEndian, int32(jBuf.Len()))
	//require.NoError(t, err)
	_, err = io.Copy(rawBuffer2, jBuf)
	took = time.Since(start)
	//require.NoError(t, err)
	t.Log("writing json", took)

	start = time.Now()
	var length2 int32
	err = binary.Read(rawBuffer2, binary.BigEndian, &length2)
	//require.NoError(t, err)
	//t.Log(length)

	var hmm2 yo

	err = json.NewDecoder(io.LimitReader(rawBuffer2, int64(length2))).Decode(&hmm2)
	took = time.Since(start)
	require.NoError(t, err)

	t.Log("json decode", took)

	//=== msgpack starts here
	start = time.Now()
	rawBuffer3 := bytes.NewBuffer(nil)
	err = binary.Write(rawBuffer3, binary.BigEndian, int32(mBuf.Len()))
	//require.NoError(t, err)
	_, err = io.Copy(rawBuffer3, mBuf)
	took = time.Since(start)
	//require.NoError(t, err)
	t.Log("writing msgpack", took)

	start = time.Now()
	var length3 int32
	err = binary.Read(rawBuffer3, binary.BigEndian, &length3)
	//require.NoError(t, err)
	//t.Log(length)

	var hmm3 yo

	err = msgpack.NewDecoder(io.LimitReader(rawBuffer3, int64(length2))).Decode(&hmm3)
	took = time.Since(start)
	require.NoError(t, err)

	t.Log("msgpack decode", took)
	t.Logf("hmm3: %+v", hmm3)

}

func TestDev(t *testing.T) {
	l, err := zap.NewDevelopment(zap.WithCaller(false))
	require.NoError(t, err)
	hey(l)
}

func hey(l *zap.Logger) {
	there(l)
}

func there(l *zap.Logger) {
	l.Warn("yo")
}

func TestMemFee(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	info := mempoolspace.NewRest()
	t.Log(info.GetFee(context.Background()))
}

func TestMemFee2(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")

	cli, err := NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)
	t.Log(cli.GetFee(context.Background()))
}

func TestMemPoolAccept(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")

	cli, err := NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)

	tx := txhelper.FromString("020000000001019f9706abf20c80969e37d1b269e1fd0245dd4c0fac1308cf30d6dfd8244cef27030000001716001492341f25a448964b67a3545ca99ab603875a550dfdffffff025fe02d0100000000225120345e4b1fed21ab5ee7fe312d64917952f4fde95d4381d0631627c17da9c24044241701000000000017a914b61071464a23ea695692ee44c8dc61bce4848bd78702473044022063a8e598c15128045084073680ade89d94de7812c91d7ed0c8071fcffda3c73b0220127524ab392c59f9e9f2d5d1f0e0cbd4eb1448f9d0a6e662b83270dbc44c6c0d0121038c69b0e713cc3f8e784c7d426ac7fe3b22912942f25151aedcdf64b49c9a1bb400000000")

	t.Log(cli.TestMemPoolAccept(context.Background(), tx))
}

func TestMemPoolUTXO(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")

	cli, err := NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)

	hash, _ := chainhash.NewHashFromStr("0da51c1bc53737fcb0cfe59cc98fa771ca8362da1b980f92cf2a664ee5eef70d")
	outpoint := wire.NewOutPoint(hash, 0)
	t.Log(cli.GetUTXO(context.Background(), *outpoint))
}

func TestGetTx(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")

	cli, err := NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)

	tx := txhelper.FromString("02000000000101fd27e384a74303b698b50397beb7bf24f58c7ff81db944a8e555bd7be9005599090000002322002072afb7c1fae3e4e73815ef8a74c4ebf323d4facb4b8b7cd37a067a85cf4b1496fdffffff01c0a61000000000001600149529b03d40b3bae6f4ad2975f870fc8dd6b28c76034730440220556b005563f80460a98ae861b3090662051966537393f137402ed8b4ff565e9e0220613d033ac13c0fffdcaab33635fc40fd784bcffb1adaa071323c2fc5a0e7c21801473044022079273f5227a190bd903157111188be61f39a5e09e7f67843bf0ddb76ebb5907702204f1917d9f70eded2601bc750c9f32f5daafdff853021cc9adb5d92f06ed4d516014e210299277201b6fb8c3a0aa527568759c7b19af3776472e0b0f2c9a1a67b1a651f86ad21026e603dbedf9a9c8bacd472b8e8feec27c3d18478e2757ab69db202972130f706ac73640380ca00b268f7f20c00")

	hash := tx.TxHash()
	t.Log(cli.GetTransaction(context.Background(), hash))
}

func TestMemPoolEntry(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	rpcUser := os.Getenv("RPC_USER")
	rpcPass := os.Getenv("RPC_PASS")
	rpcHost := os.Getenv("RPC_HOST")

	cli, err := NewClient(rpcHost, rpcUser, rpcPass)
	require.NoError(t, err)

	tx := txhelper.FromString("020000000001019f9706abf20c80969e37d1b269e1fd0245dd4c0fac1308cf30d6dfd8244cef27030000001716001492341f25a448964b67a3545ca99ab603875a550dfdffffff025fe02d0100000000225120345e4b1fed21ab5ee7fe312d64917952f4fde95d4381d0631627c17da9c24044241701000000000017a914b61071464a23ea695692ee44c8dc61bce4848bd78702473044022063a8e598c15128045084073680ade89d94de7812c91d7ed0c8071fcffda3c73b0220127524ab392c59f9e9f2d5d1f0e0cbd4eb1448f9d0a6e662b83270dbc44c6c0d0121038c69b0e713cc3f8e784c7d426ac7fe3b22912942f25151aedcdf64b49c9a1bb400000000")

	t.Log(cli.GetMemPoolEntry(context.Background(), tx.TxHash()))
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
