package main

import (
	"log"
	"sync"
	"time"
)

var snowflake *Snowflake

var sonce sync.Once

func initSnowflake(p Provider) *Snowflake {
	sonce.Do(func() {
		snowflake = &Snowflake{provider: p}
	})
	return snowflake
}

type Snowflake struct {
	sync.Mutex       // 锁
	timestamp  int64 // 时间戳 ，毫秒
	//workerId     int64 // 工作节点
	datacenterId int64 // 数据中心机房id
	sequence     int64 // 序列号
	provider     Provider
}

const (
	epoch             = int64(1640966400000)                           // 设置起始时间(时间戳/毫秒)：2020-01-01 00:00:00，有效期69年
	timestampBits     = uint(41)                                       // 时间戳占用位数
	datacenterIdBits  = uint(2)                                        // 数据中心id所占位数
	workerIdBits      = uint(8)                                        // 机器id所占位数
	sequenceBits      = uint(12)                                       // 序列所占的位数
	timestampMax      = int64(-1 ^ (-1 << timestampBits))              // 时间戳最大值
	maxDatacenterId   = int64(-1 ^ (-1 << datacenterIdBits))           // 支持的最大数据中心id数量
	maxWorkerId       = int64(-1 ^ (-1 << workerIdBits))               // 支持的最大机器id数量
	sequenceMask      = int64(-1 ^ (-1 << sequenceBits))               // 支持的最大序列id数量
	workerIdShift     = sequenceBits                                   // 机器id左移位数
	datacenterIdShift = sequenceBits + workerIdBits                    // 数据中心id左移位数
	timestampShift    = sequenceBits + workerIdBits + datacenterIdBits // 时间戳左移位数
)

func (s *Snowflake) NextId() int64 {
	s.Lock()
	defer s.Unlock()
	now := time.Now().UnixMilli()
	if s.timestamp == now {
		// 当同一时间戳（精度：毫秒）下多次生成id会增加序列号
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			// 如果当前序列超出12bit长度，则需要等待下一毫秒
			// 下一毫秒将使用sequence:0
			for now <= s.timestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		// 不同时间戳（精度：毫秒）下直接使用序列号：0
		s.sequence = 0
	}
	t := now - epoch
	if t > timestampMax {
		log.Printf("epoch must be between 0 and %d\n", timestampMax-1)
		return 0
	}
	s.timestamp = now
	workerId, err := s.getWorkerId()
	if err != nil {
		log.Printf("get workerId error %v\n", err)
		return 0
	}
	r := (t)<<timestampShift | (s.datacenterId << datacenterIdShift) | (workerId << workerIdShift) | (s.sequence)
	return r
}

func (s *Snowflake) getWorkerId() (int64, error) {
	return s.provider.GetWorkerId()
}
