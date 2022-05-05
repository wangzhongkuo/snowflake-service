package main

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"strconv"
	"sync"
	"time"
)

var once sync.Once
var simpleProvider *SimpleProvider
var consulProvider *ConsulProvider

type Provider interface {
	GetWorkerId() (int64, error)
}

type SimpleProvider struct {
	workerId int64
}

func getSimpleProvider(workerId int64) *SimpleProvider {
	if workerId < 0 || workerId > maxWorkerId {
		panic(fmt.Sprintf("workerId must beetwen 0 and %d", maxWorkerId))
	}
	once.Do(func() {
		simpleProvider = &SimpleProvider{
			workerId: workerId,
		}
	})
	return simpleProvider
}

func (p *SimpleProvider) GetWorkerId() (int64, error) {
	return p.workerId, nil
}

func getConsulProvider(Address string, keyPrefix string) *ConsulProvider {
	once.Do(func() {
		leaderCh := make(chan struct{}, 1)
		leaderCh <- struct{}{} // used by start
		consulProvider = &ConsulProvider{
			Address:   Address,
			keyPrefix: keyPrefix,
			stopCh:    make(<-chan struct{}),
			leaderCh:  leaderCh,
			state:     unavailable,
		}
		go consulProvider.start()
	})
	return consulProvider
}

type state int64

const (
	unavailable = state(0)
	available   = state(1)
)

type ConsulProvider struct {
	sync.Mutex
	Address   string
	leaderCh  <-chan struct{}
	workerId  int64
	keyPrefix string
	stopCh    <-chan struct{}
	state     state
}

func (p *ConsulProvider) GetWorkerId() (int64, error) {
	if p.state == unavailable {
		return 0, fmt.Errorf("consulProvider is unavailable")
	} else {
		return p.workerId, nil
	}
}

func (p *ConsulProvider) start() {
	for {
		select {
		case <-p.leaderCh:
			p.state = unavailable
			config := api.DefaultConfig()
			config.Address = p.Address
			c, _ := api.NewClient(config)
			var workerId int64
			for ; workerId <= maxWorkerId; workerId++ {
				lockOptions := &api.LockOptions{
					Key:          p.keyPrefix + strconv.FormatInt(workerId, 10),
					LockTryOnce:  true,
					LockWaitTime: time.Millisecond,
				}
				lock, _ := c.LockOpts(lockOptions)
				ch, err := lock.Lock(p.stopCh)
				if ch != nil && err == nil {
					p.leaderCh = ch
					p.workerId = workerId
					p.state = available
					break
				} else {
					log.Println("create lock error", err)
					if workerId == maxWorkerId {
						leaderCh := make(chan struct{}, 1)
						leaderCh <- struct{}{}
						p.leaderCh = leaderCh
					}
				}
				time.Sleep(3 * time.Second)
			}
		}
	}
}
