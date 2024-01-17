package network

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Dialer func(network, addr string) (c net.Conn, err error)

type Opts struct {
	//::builder-gen -with-globals -prefix=With -no-builder
	Dialer *Dialer
}
type Network struct {
	addressManager   *AddressManager
	params           *chaincfg.Params
	opts             Opts
	seenTransactions map[chainhash.Hash]int64
}

func New(p *chaincfg.Params, fns ...OptsFunc) *Network {
	return &Network{params: p, opts: ToOpts(fns...), addressManager: NewAddressManager(p)}
}

func (n *Network) Connect() {
	var mng *grabber.TransactionManager
	var err error
	if n.params == &chaincfg.TestNet3Params {
		mng, err = grabber.DefaultManager()
	} else {
		mng, err = grabber.MainNetManager()
	}
	if err != nil {
		panic(err)
	}
	cmgr, err := connmgr.New(&connmgr.Config{
		TargetOutbound: 300,
		RetryDuration:  5 * time.Second,
		GetNewAddress:  n.addressManager.NewAddress,
		Dial: func(addr net.Addr) (net.Conn, error) {
			return n.dial(addr.String())
		},
		OnConnection: func(req *connmgr.ConnReq, conn net.Conn) {
			//req.State()
			//log.Println("connecting to", req.Addr)
			p, err := peer.NewOutboundPeer(&peer.Config{
				NewestBlock:       mng.GetBestBlock,
				HostToNetAddress:  nil,
				Proxy:             "",
				UserAgentName:     "",
				UserAgentVersion:  "",
				UserAgentComments: nil,
				ChainParams:       nil,
				Services:          wire.SFNodeNetwork,
				ProtocolVersion:   peer.MaxProtocolVersion,
				DisableRelayTx:    false,
				Listeners: peer.MessageListeners{
					OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
						p.QueueMessage(wire.NewMsgGetAddr(), nil)
					},
					OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
						for _, a := range msg.AddrList {
							addr := fmt.Sprintf("%s:%d", a.IP.String(), a.Port)
							fmt.Println("new peer found:", addr)
							n.addressManager.AddAddress(
								addr,
							)
						}
					},
				},
				TrickleInterval:     0,
				AllowSelfConns:      false,
				DisableStallHandler: false,
			}, req.Addr.String())

			if err != nil {
				panic(err)
				os.Exit(1)
			}

			p.AssociateConnection(conn)
			go func() {
				p.WaitForDisconnect()
				fmt.Println("peer disconnected", conn.RemoteAddr())
			}()
		},
	})
	if err != nil {
		panic(err)
	}

	connmgr.SeedFromDNS(n.params, wire.SFNodeNetwork|wire.SFNode2X, defaultIPLookup,
		n.addressManager.OnSeed)

	time.Sleep(5 * time.Second)

	go cmgr.Start()

	select {}

	//var err error
	//var mngr *grabber.TransactionManager
	//switch n.params {
	//case &chaincfg.MainNetParams:
	//	mngr, err = grabber.MainNetManager()
	//case &chaincfg.TestNet3Params:
	//	mngr, err = grabber.DefaultManager()
	//}
	//if err != nil {
	//	return err
	//}
	//n.mngr = mngr
	//return nil

}

func (n *Network) dial(addr string) (conn net.Conn, err error) {
	if n.opts.HasDialer() {
		dial := *n.opts.Dialer
		conn, err = dial("tcp", addr)
	} else {
		conn, err = net.Dial("tcp", addr)
	}

	_ = connmgr.ConnManager{}

	return
}

func (n *Network) connectToPeer(addr string) (*peer.Peer, error) {
	verack := make(chan struct{})
	p, err := peer.NewOutboundPeer(&peer.Config{
		HostToNetAddress:  nil,
		Proxy:             "",
		UserAgentName:     "test-client",
		UserAgentVersion:  "0.0.0",
		UserAgentComments: nil,
		ChainParams:       &chaincfg.TestNet3Params,
		Services:          1024,
		ProtocolVersion:   70015,
		DisableRelayTx:    false,
		Listeners: peer.MessageListeners{
			OnVersion: func(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
				fmt.Println("outbound: received version")
				return nil
			},
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				verack <- struct{}{}
			},
			OnTx: func(p *peer.Peer, msg *wire.MsgTx) {
				fmt.Println("got a transaction", msg.TxHash())
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				log.Println("got an invoice", len(msg.InvList), "messages")
				for _, m := range msg.InvList {
					log.Println(m.Type, m.Hash)
				}
			},
			OnGetData: func(p *peer.Peer, msg *wire.MsgGetData) {
				log.Println("peer asking for data")
				for _, m := range msg.InvList {
					log.Println(m.Type, m.Hash)
				}
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				//log.Println("got addresses from peer", len(msg.AddrList))
				//for _, n := range msg.AddrList {
				//	log.Printf("%+v", n)
				//}

			},
		},
		TrickleInterval:     500 * time.Millisecond,
		AllowSelfConns:      true,
		DisableStallHandler: false,
	}, addr)
	if err != nil {
		return nil, err
	}
	conn, err := n.dial(p.Addr())

	p.AssociateConnection(conn)

	go func() {
		select {
		case <-verack:
			//p.QueueMessage(wire.NewMsgGetAddr(), nil)
			//select {}

			//case <-time.After(time.Second * 100):
			//	fmt.Printf("Example_peerConnection: verack timeout")
		}
	}()

	return p, nil
}

func defaultIPLookup(host string) ([]net.IP, error) {
	if strings.HasSuffix(host, ".onion") {
		return nil, fmt.Errorf("attempt to resolve tor address %s", host)
	}

	return net.LookupIP(host)
}

// onionAddr implements the net.Addr interface and represents a tor address.
type onionAddr struct {
	addr string
}

// String returns the onion address.
//
// This is part of the net.Addr interface.
func (oa *onionAddr) String() string {
	return oa.addr
}

// Network returns "onion".
//
// This is part of the net.Addr interface.
func (oa *onionAddr) Network() string {
	return "onion"
}

func addrStringToNetAddr(addr string) (net.Addr, error) {
	host, strPort, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return nil, err
	}

	// Skip if host is already an IP address.
	if ip := net.ParseIP(host); ip != nil {
		return &net.TCPAddr{
			IP:   ip,
			Port: port,
		}, nil
	}

	// Tor addresses cannot be resolved to an IP, so just return an onion
	// address instead.
	if strings.HasSuffix(host, ".onion") {
		//if cfg.NoOnion {
		return nil, errors.New("tor has been disabled")
		//}

		//return &onionAddr{addr: addr}, nil
	}

	// Attempt to look up an IP address associated with the parsed host.
	ips, err := defaultIPLookup(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", host)
	}

	return &net.TCPAddr{
		IP:   ips[0],
		Port: port,
	}, nil
}
