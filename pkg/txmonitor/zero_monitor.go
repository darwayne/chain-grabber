package txmonitor

import (
	"context"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/go-zeromq/zmq4"
	"github.com/pkg/errors"
	"log"
	"strings"
	"sync"
	"time"
)

func NewZeroMonitor(host string) ZeroMonitor {
	b := broadcaster.NewBroker[*wire.MsgTx]()
	go b.Start()
	if !strings.HasPrefix(host, "tcp://") {
		host = "tcp://" + host
	}
	return ZeroMonitor{host: host, broker: b}
}

type ZeroMonitor struct {
	host   string
	broker *broadcaster.Broker[*wire.MsgTx]
}

func (m *ZeroMonitor) Subscribe() chan *wire.MsgTx {
	return m.broker.Subscribe()
}

func (m *ZeroMonitor) UnSubscribe(channel chan *wire.MsgTx) {
	m.broker.UnSubscribe(channel)
}

func (m *ZeroMonitor) Stop() {
	m.broker.Stop()
}

func (m *ZeroMonitor) Start(ctx context.Context) error {
	sub := zmq4.NewSub(ctx)
	err := sub.Dial(m.host)
	if err != nil {
		return errors.Wrap(err, "could not dial")
	}

	err = sub.SetOption(zmq4.OptionSubscribe, "rawtx")
	if err != nil {
		return errors.Wrap(err, "could not subscribe")
	}

	var mu sync.RWMutex
	var lastMessageSentAt time.Time

	doneChan := make(chan struct{})

	defer func() {
		close(doneChan)
	}()

	go func() {
		tick := time.NewTicker(10 * time.Minute)
		for {
			select {
			case <-doneChan:
				return
			case <-tick.C:
				currentTime := time.Now()
				mu.RLock()
				myTime := lastMessageSentAt
				mu.RUnlock()

				if currentTime.Sub(myTime) > 10*time.Minute {
					log.Println("[WARN] mempool has not seen a message in 10 minutes")
				}
			}
		}
	}()

	for {
		// Read envelope
		msg, err := sub.Recv()
		if err != nil {
			return errors.Wrap(err, "could not receive message")
		}

		if len(msg.Frames) < 2 {
			return errors.New("unexpected message frames")
		}
		tx := txhelper.FromBytes(msg.Frames[1])
		if tx == nil {
			return errors.New("bad tx detected")
		}

		m.broker.Publish(tx)
		mu.Lock()
		lastMessageSentAt = time.Now()
		mu.Unlock()
	}
}
