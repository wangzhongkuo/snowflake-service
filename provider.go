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
	Stop()
}

type SimpleProvider struct {
	workerId int64
}

func (p *SimpleProvider) Stop() {}

func getSimpleProvider(workerId int64) *SimpleProvider {
	if workerId < 0 || workerId > maxWorkerId {
		log.Fatalf("workerId must between 0 and %d", maxWorkerId)
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

func getConsulProvider(Address string, keyPrefix string, hintWorkerId int64) *ConsulProvider {
	once.Do(func() {
		leaderCh := make(chan struct{}, 1)
		leaderCh <- struct{}{} // used by start
		consulProvider = &ConsulProvider{
			Address:      Address,
			keyPrefix:    keyPrefix,
			hintWorkerId: hintWorkerId,
			leaderCh:     leaderCh,
			stopCh:       make(chan struct{}, 1),
			state:        unavailable,
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
	Address      string
	lock         *api.Lock
	leaderCh     <-chan struct{}
	stopCh       chan struct{}
	hintWorkerId int64
	workerId     int64
	keyPrefix    string
	state        state
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
			workerId := roundPre(p.hintWorkerId, maxWorkerId)
			var i int64
			for i = 0; i <= maxWorkerId; i++ {
				workerId = roundNext(workerId, maxWorkerId)
				log.Printf("start accquire worker id: %d", workerId)
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
					p.lock = lock
					log.Printf("accquire worker id success: %d", workerId)
					break
				} else {
					log.Printf("accquire worker id %d error %v", workerId, err)
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

func (p *ConsulProvider) Stop() {
	log.Printf("ConsulProvider stop, release worker id: %d", p.workerId)
	p.stopCh <- struct{}{}
	p.lock.Unlock()
}

// roundNext get the next value with round-robin algorithm
// Note: contains zero, e.g. pos = 3, max = 5, the result will be [4, 5, 0, 1, 2, 3, 4, 5, 0...]
func roundNext(pos, max int64) int64 {
	next := pos + 1
	if next > max {
		return 0
	}
	return next
}

// roundPre get the previous value with reverse-round-robin algorithm
// Note: contains zero, e.g. pos = 3, max = 5, the result will be [2, 1, 0, 5, 4, 3, 2, 1, 0...]
func roundPre(pos, max int64) int64 {
	pre := pos - 1
	if pre < 0 {
		return max
	}
	return pre
}
