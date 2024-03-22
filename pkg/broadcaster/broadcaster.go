package broadcaster

import (
	"context"
	"go.uber.org/zap"
	"sync"
	"sync/atomic"
	"time"
)

type BroadCaster struct {
	isTestNet bool
	Broker    *Broker[*string]

	connected map[string]struct{}
	ignored   map[string]struct{}
	logger    *zap.Logger
}

func New(isTestNet bool, logger *zap.Logger) *BroadCaster {
	b := &BroadCaster{
		isTestNet: isTestNet,
		connected: make(map[string]struct{}),
		ignored:   make(map[string]struct{}),
		Broker:    NewBroker[*string](),
		logger:    logger,
	}
	go b.Broker.Start()
	return b
}

func (b *BroadCaster) Connect(ctx context.Context) {
	broker := b.Broker
	logger := b.logger
	connected := make(chan *ElectrumClient, 1)
	go b.generateClients(ctx, connected)
	for cli := range connected {
		server := cli.Server
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			sub := broker.Subscribe()
			for {
				select {
				case <-ctx.Done():
					return
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

func (b *BroadCaster) generateClients(ctx context.Context, channel chan *ElectrumClient) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			close(channel)
		}
	}()

	known := make(map[string]struct{})
	var addresses []string
	if b.isTestNet {
		addresses = append(addresses, knownTestNetElectrumNodes...)
	} else {
		addresses = append(addresses, knownMainNetElectrumNodes...)
	}

	const maxClients = 2_000
	var mu sync.RWMutex
	sem := make(chan struct{}, 4)
	var connected int64

	var connect func(string)

	connect = func(server string) {
		if atomic.LoadInt64(&connected) >= maxClients {

			return
		}
		sem <- struct{}{}
		defer func() {
			<-sem
		}()

		cli := NewElectrumClient()
		if err := cli.Connect(server); err != nil {
			//logger.Warn("error connecting", zap.Error(err),
			//	zap.String("server", server))
			return
		}

		val := atomic.AddInt64(&connected, 1)

		b.logger.Info("electrum client connected",
			zap.String("server", server),
			zap.Int64("connected", val),
		)

		select {
		case channel <- cli:
		case <-ctx.Done():
			return
		}

		peers, _ := cli.GetPeers()
		for _, p := range peers {
			s := p.SSLServer()
			if s == "" {
				continue
			}
			mu.Lock()
			_, found := known[s]
			if found {
				mu.Unlock()
				continue
			}
			known[s] = struct{}{}
			mu.Unlock()
			go connect(s)
		}

	}

	for _, address := range addresses {
		mu.Lock()
		known[address] = struct{}{}
		mu.Unlock()
		go connect(address)
	}

	<-ctx.Done()
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
