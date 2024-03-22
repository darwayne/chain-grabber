package main

import (
	"context"
	"flag"
	"github.com/darwayne/chain-grabber/cmd/grabby/internal/passwords"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/memspender"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/darwayne/chain-grabber/pkg/txmonitor"
	"go.uber.org/zap"
	"os"
)

func main() {
	address := flag.String("address", "", "the address to send funds to")
	isTestNet := flag.Bool("test-net", true, "whether to connect to testnet or not")
	proxyUser := flag.String("proxy-user", passwords.User, "proxy user to use")
	proxyPass := flag.String("proxy-pass", passwords.Pass, "proxy pass to use")
	keyStart := flag.Int("key-start", 1, "the private key to start with")
	keyEnd := flag.Int("key-end", 100_000, "the private key to end with")
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
	if isMainNet {
		n, err = lightnode.NewMainNet(l)
	} else {
		n, err = lightnode.NewTestNet(l)
	}
	if err != nil {
		panic(err)
	}

	errChan := make(chan error, 1)
	spender, err := memspender.New(m.Subscribe(), !isMainNet, getPublisher(!isMainNet, l), *address, l)
	if err != nil {
		panic(err)
	}

	go func() {
		l.Info("generating keys")
		err := spender.GenerateKeys([2]int{*keyStart, *keyEnd})
		if err != nil {
			l.Warn("error generating keys", zap.String("err", err.Error()))
			errChan <- err
			return
		}

		l.Info("key generation complete!")

		go spender.Start(ctx)

		go n.ConnectV2()
		go m.Start(ctx, n)
	}()

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
