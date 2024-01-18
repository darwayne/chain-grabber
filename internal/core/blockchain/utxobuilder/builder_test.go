package utxobuilder

import (
	"context"
	"github.com/darwayne/chain-grabber/internal/core/blockchain"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/proxy"
	"net"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestBuilder(t *testing.T) {
	store := NewUTXOStore("./testdata/mainnet.gob.gz")
	b := NewBuilder(getClients(), store)

	err := b.Build()
	require.NoError(t, err)
}

func getClients() []blockchain.Client {
	user := os.Getenv("PROXY_USER")
	pass := os.Getenv("PROXY_PASS")

	hosts := []string{
		"iad.socks.ipvanish.com",
		"atl.socks.ipvanish.com",
		"chi.socks.ipvanish.com",
		"dal.socks.ipvanish.com",
		"den.socks.ipvanish.com",
		"lax.socks.ipvanish.com",
		"mia.socks.ipvanish.com",
		"nyc.socks.ipvanish.com",
		"phx.socks.ipvanish.com",
		"sea.socks.ipvanish.com",
		"tor.socks.ipvanish.com",
		"syd.socks.ipvanish.com",
		"par.socks.ipvanish.com",
		"fra.socks.ipvanish.com",
		"lin.socks.ipvanish.com",
		"nrt.socks.ipvanish.com",
		"ams.socks.ipvanish.com",
		"waw.socks.ipvanish.com",
		"lis.socks.ipvanish.com",
		"sin.socks.ipvanish.com",
		"mad.socks.ipvanish.com",
		"sto.socks.ipvanish.com",
		"lon.socks.ipvanish.com",
	}

	var clients []blockchain.Client
	for _, host := range hosts {
		dialer, err := proxy.SOCKS5("tcp", host+":1080",
			&proxy.Auth{
				User:     user,
				Password: pass,
			},
			proxy.Direct)

		if err != nil {
			panic(err)
		}
		dialContext := func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}

		cli := newClient(dialContext)
		r := mempoolspace.NewRest(mempoolspace.WithHttpClient(*cli))
		clients = append(clients, r)
	}

	return clients
}

type DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

func newClient(dialContext DialContext) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           dialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
		},
	}
}
