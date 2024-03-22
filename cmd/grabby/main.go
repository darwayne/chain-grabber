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
	"golang.org/x/sync/errgroup"
	"os"
	"sync"
	"time"
)

func main() {
	address := flag.String("address", "", "the address to send funds to")
	isTestNet := flag.Bool("test-net", true, "whether to connect to testnet or not")
	proxyUser := flag.String("proxy-user", passwords.User, "proxy user to use")
	proxyPass := flag.String("proxy-pass", passwords.Pass, "proxy pass to use")
	keyStart := flag.Int("key-start", 0, "the private key to start with")
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
	broker := broadcaster.NewBroker[*string]()
	go broker.Start()
	addClientsToBroker(broker, isTestNet, logger)

	return broker
}

func addClientsToBroker(broker *broadcaster.Broker[*string], isTestNet bool, logger *zap.Logger) {
	for _, c := range electrumClients(isTestNet, logger) {
		cli := c
		server := c.Server
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			sub := broker.Subscribe()
			for {
				select {
				case msg := <-sub:
					res, err := cli.Broadcast(*msg)
					if err != nil {
						logger.Warn("error broadcasting to node", zap.Error(err), zap.String("server", server))
					} else {
						logger.Info("successfully broadcasted", zap.String("txid", res),
							zap.String("server", server))
					}
				case <-ticker.C:
					_, err := cli.Ping()
					if err != nil {
						logger.Warn("error pinging reconnecting", zap.String("server", server))
						cli.Reconnect()
					} else {
						//logger.Info("ping success", zap.String("server", server))
					}

				}
			}
		}()
	}
}

func electrumClients(isTestnet bool, logger *zap.Logger) []*broadcaster.ElectrumClient {
	var addresses []string
	if isTestnet {
		addresses = append(addresses, knownTestNetElectrumNodes...)
	} else {
		addresses = append(addresses, knownMainNetElectrumNodes...)
	}

	var mu sync.RWMutex
	var clients []*broadcaster.ElectrumClient
	var group errgroup.Group

	for _, address := range addresses {
		server := address
		group.Go(func() error {
			cli := broadcaster.NewElectrumClient()
			if err := cli.Connect(server); err != nil {
				logger.Warn("error connecting", zap.Error(err),
					zap.String("server", server))
				return nil
				//return err
			}

			mu.Lock()
			clients = append(clients, cli)
			mu.Unlock()

			return nil
		})
	}

	err := group.Wait()
	if err != nil {
		panic(err)
	}

	return clients
}

var knownTestNetElectrumNodes = []string{
	"testnet.qtornado.com:51002",
	"v22019051929289916.bestsrv.de:50002",
	//"testnet.hsmiths.com:53012",
	"blockstream.info:993",
	"electrum.blockstream.info:60002",
	"testnet.aranguren.org:51002",
}

var knownMainNetElectrumNodes = []string{
	"xtrum.com:50002",
	"blockstream.info:700",
	"electrum.bitaroo.net:50002",
	"electrum0.snel.it:50002",
	"btc.electroncash.dk:60002",
	"e.keff.org:50002",
	//"2electrumx.hopto.me:56022",
	"blkhub.net:50002",
	"bolt.schulzemic.net:50002",
	//"vmd71287.contaboserver.net:50002",
	"smmalis37.ddns.net:50002",
	"electrumx.alexridevski.net:50002",
	//"f.keff.org:50002",
	"mainnet.foundationdevices.com:50002",
	//"assuredly.not.fyi:50002",
	//"vmd104014.contaboserver.net:50002",
	//"ex05.axalgo.com:50002",
	//"exs.ignorelist.com:50002",
	"eai.coincited.net:50002",
	//"2ex.digitaleveryware.com:50002",
}
