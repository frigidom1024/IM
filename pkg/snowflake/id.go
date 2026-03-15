package snowflake

import (
	"fmt"
	"sync"
	"time"
)

const (
	// 时间起始点 2024-01-01 00:00:00
	epoc int64 = 1704067200000

	// 数据中心ID位数
	datacenterIDBits = 5
	// 机器ID位数
	workerIDBits = 5
	// 序列号位数
	sequenceBits = 12

	// 最大值
	maxDatacenterID = -1 ^ (-1 << datacenterIDBits)
	maxWorkerID     = -1 ^ (-1 << workerIDBits)
	maxSequence     = -1 ^ (-1 << sequenceBits)

	// 位移
	workerIDShift     = sequenceBits
	datacenterIDShift = sequenceBits + workerIDBits
	timestampShift    = sequenceBits + workerIDBits + datacenterIDBits
)

// IDGenerator 雪花ID生成器
type IDGenerator struct {
	mu           sync.Mutex
	datacenterID int64
	workerID     int64
	sequence     int64
	lastTime     int64
}

var generator *IDGenerator

// Init 初始化ID生成器
func Init(datacenterID, workerID int64) error {
	if datacenterID < 0 || datacenterID > maxDatacenterID {
		return fmt.Errorf("datacenter ID must be between 0 and %d", maxDatacenterID)
	}
	if workerID < 0 || workerID > maxWorkerID {
		return fmt.Errorf("worker ID must be between 0 and %d", maxWorkerID)
	}

	generator = &IDGenerator{
		datacenterID: datacenterID,
		workerID:     workerID,
	}

	return nil
}

// NextID 生成下一个ID
func NextID() (int64, error) {
	if generator == nil {
		return 0, fmt.Errorf("IDGenerator not initialized")
	}

	generator.mu.Lock()
	defer generator.mu.Unlock()

	now := time.Now().UnixMilli()

	if now == generator.lastTime {
		generator.sequence = (generator.sequence + 1) & maxSequence
		if generator.sequence == 0 {
			// 序列号溢出，等待下一毫秒
			for now <= generator.lastTime {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		generator.sequence = 0
	}

	generator.lastTime = now

	// 组装ID
	id := ((now - int64(epoc)) << timestampShift) |
		(generator.datacenterID << datacenterIDShift) |
		(generator.workerID << workerIDShift) |
		generator.sequence

	return id, nil
}

// MustNextID 生成下一个ID，失败则panic
func MustNextID() int64 {
	id, err := NextID()
	if err != nil {
		panic(err)
	}
	return id
}
