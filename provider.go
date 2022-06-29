package main

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
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

func getConsulProvider(address string, keyPrefix string, hintWorkerId int64, enableSelfPreservation bool) *ConsulProvider {
	if hintWorkerId < 0 || hintWorkerId > maxWorkerId {
		log.Printf("hint-worker-id must between 0 and %d, use default 0\n", maxWorkerId)
		hintWorkerId = 0
	}
	once.Do(func() {
		leaderCh := make(chan struct{}, 1)
		leaderCh <- struct{}{} // used by start
		workerId := atomic.Value{}
		workerId.Store(hintWorkerId)
		state := atomic.Value{}
		state.Store(unavailable)
		consulProvider = &ConsulProvider{
			Address:                address,
			keyPrefix:              keyPrefix,
			workerId:               workerId,
			leaderCh:               leaderCh,
			stopCh:                 make(chan struct{}),
			state:                  state,
			enableSelfPreservation: enableSelfPreservation,
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
	Address                string
	lock                   *api.Lock
	leaderCh               <-chan struct{}
	stopCh                 chan struct{}
	workerId               atomic.Value
	keyPrefix              string
	state                  atomic.Value
	enableSelfPreservation bool
}

func (p *ConsulProvider) GetWorkerId() (int64, error) {
	if p.state.Load() == unavailable {
		if !p.enableSelfPreservation {
			log.Printf("Fatal: enable-self-preservation=%v, consul provider is unavailable!!!", p.enableSelfPreservation)
			return 0, fmt.Errorf("consulProvider is unavailable")
		} else {
			workerId := p.workerId.Load().(int64)
			log.Printf("Warnning: enable-self-preservation=%v, consul provider is unavailable, use worker id: %d", p.enableSelfPreservation, workerId)
			return workerId, nil
		}
	}
	return p.workerId.Load().(int64), nil
}

func (p *ConsulProvider) start() {
	for {
		select {
		case <-p.stopCh:
			return
		case <-p.leaderCh:
			p.state.Store(unavailable)
			config := api.DefaultConfig()
			config.Address = p.Address
			c, _ := api.NewClient(config)
			workerId := roundPre(p.workerId.Load().(int64), maxWorkerId)
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
					p.workerId.Store(workerId)
					p.state.Store(available)
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
	defer func() {
		if err := recover(); err != nil {
			log.Println("stop consul provider err: ", err)
		}
	}()
	log.Printf("ConsulProvider stop, release worker id: %d", p.workerId)
	close(p.stopCh)
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
