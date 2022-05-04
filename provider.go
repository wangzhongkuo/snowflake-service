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
var defaultProvider *DefaultProvider
var provider *ConsulProvider

type Provider interface {
	GetWorkerId() (int64, error)
}

type DefaultProvider struct {
	workerId int64
}

func getDefaultProvider(workerId int64) *DefaultProvider {
	if workerId < 0 || workerId > maxWorkerId {
		panic(fmt.Sprintf("workerId must beetwen 0 and %d", maxWorkerId))
	}
	once.Do(func() {
		defaultProvider = &DefaultProvider{
			workerId: workerId,
		}
	})
	return defaultProvider
}

func (p *DefaultProvider) GetWorkerId() (int64, error) {
	return p.workerId, nil
}

func getConsulProvider(Address string, keyPrefix string) *ConsulProvider {
	once.Do(func() {
		leaderCh := make(chan struct{}, 1)
		leaderCh <- struct{}{} // used by start
		provider = &ConsulProvider{
			Address:   Address,
			keyPrefix: keyPrefix,
			stopCh:    make(<-chan struct{}),
			leaderCh:  leaderCh,
			state:     unavailable,
		}
		go provider.start()
	})
	return provider
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
	if p.getState() == unavailable {
		return 0, fmt.Errorf("provider is unavailable")
	} else {
		return p.getWorkerId(), nil
	}
}

func (p *ConsulProvider) getLeaderCh() <-chan struct{} {
	p.Lock()
	defer p.Unlock()
	return p.leaderCh
}

func (p *ConsulProvider) setLeaderCh(leaderCh <-chan struct{}) {
	p.Lock()
	defer p.Unlock()
	p.leaderCh = leaderCh
}

func (p *ConsulProvider) getWorkerId() int64 {
	p.Lock()
	defer p.Unlock()
	return p.workerId
}

func (p *ConsulProvider) setWorkerId(workerId int64) {
	p.Lock()
	defer p.Unlock()
	p.workerId = workerId
}

func (p *ConsulProvider) getState() state {
	p.Lock()
	defer p.Unlock()
	return p.state
}

func (p *ConsulProvider) setState(state state) {
	p.Lock()
	defer p.Unlock()
	p.state = state
}

func (p *ConsulProvider) start() {
	for {
		select {
		case <-p.getLeaderCh():
			p.setState(unavailable)
			c, _ := api.NewClient(api.DefaultConfig())
			var workerId int64
			for ; workerId <= maxWorkerId; workerId++ {
				lockOptions := &api.LockOptions{
					Key:          fmt.Sprintf("%s%s", p.keyPrefix, strconv.FormatInt(workerId, 10)),
					LockTryOnce:  true,
					LockWaitTime: time.Millisecond,
				}
				lock, _ := c.LockOpts(lockOptions)
				ch, err := lock.Lock(p.stopCh)
				if ch != nil && err == nil {
					p.setLeaderCh(ch)
					p.setWorkerId(workerId)
					p.setState(available)
					break
				} else {
					log.Println("create lock error", err)
					if workerId == maxWorkerId {
						leaderCh := make(chan struct{}, 1)
						leaderCh <- struct{}{}
						p.setLeaderCh(leaderCh)
					}
				}
				time.Sleep(3 * time.Second)
			}
		}
	}
}
