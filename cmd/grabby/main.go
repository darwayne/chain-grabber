package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/darwayne/chain-grabber/cmd/grabby/internal/passwords"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/noderpc"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/keygen"
	"github.com/darwayne/chain-grabber/pkg/memspender"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/darwayne/chain-grabber/pkg/txmonitor"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"time"
)

func main() {
	address := flag.String("address", "", "the address to send funds to")
	isTestNet := flag.Bool("test-net", true, "whether to connect to testnet or not")
	proxyUser := flag.String("proxy-user", passwords.User, "proxy user to use")
	proxyPass := flag.String("proxy-pass", passwords.Pass, "proxy pass to use")
	secretDir := flag.String("secrets-dir", "./grabby-db", "the directory where leveldb secrets are stored")
	flag.Parse()

	os.Setenv("PROXY_USER", *proxyUser)
	os.Setenv("PROXY_PASS", *proxyPass)

	if *address == "" {
		panic("address required")
	}

	m := txmonitor.New()
	ctx := context.Background()

	l, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	isMainNet := !*isTestNet
	var n *lightnode.Node
	var params *chaincfg.Params
	var cli memspender.NetworkGrabber
	if isMainNet {
		params = &chaincfg.MainNetParams
		n, err = lightnode.NewMainNet(l)
		if err == nil {
			cli, err = noderpc.NewClient(passwords.RPCHost,
				passwords.RPCUser, passwords.RPCPass)
		}
	} else {
		params = &chaincfg.TestNet3Params
		n, err = lightnode.NewTestNet(l)
		if err == nil {
			cli = mempoolspace.NewRest(mempoolspace.WithNetwork(params))
		}
	}
	if err != nil {
		panic(err)
	}

	fmt.Println("running tx health check")
	hash, err := chainhash.NewHashFromStr("affab9fcd998c8bcc4621faa76b7dc01fc5ff9c2c4c9c74ba3248880ab20ddb3")
	if err != nil {
		panic(err)
	}
	start := time.Now()
	result, err := cli.GetTransaction(context.Background(), *hash)
	took := time.Since(start)
	fmt.Println("tx health check completed in", took,
		"result:", result, "err", err)

	errChan := make(chan error, 1)
	//mempoolspace.NewRest(mempoolspace.WithNetwork(params))
	spender, err := memspender.New(m.Subscribe(), !isMainNet, getPublisher(!isMainNet, l), *address, l, cli)
	if err != nil {
		panic(err)
	}

	info := filepath.Join(*secretDir, "keydbv2.sqlite")
	db, err := sql.Open("sqlite3", info)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	secretStore := keygen.NewSQLReader(db, params)
	l.Info("running health check", zap.String("db", info))
	if err := secretStore.HealthCheck(); err != nil {
		l.Error("failed health check", zap.Error(err))
		return
	}
	l.Info("health check passed")
	spender.SetSecrets(secretStore)

	go spender.Start(ctx)

	go n.ConnectV2()
	go m.Start(ctx, n)

	l.Info("INITIALIZED",
		zap.String("address", *address),
		zap.Bool("isMainNet", isMainNet),
	)

	select {
	case err := <-errChan:
		panic(err)
	case <-sigutil.Done():
	}
}

func getPublisher(isTestNet bool, logger *zap.Logger) *broadcaster.Broker[*string] {
	broker := broadcaster.New(isTestNet, logger)
	go broker.Connect(context.Background())

	return broker.Broker
}
