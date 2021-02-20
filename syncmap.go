package syncmap

import (
	"container/list"
	"sync"
	"time"
)

type Value struct {
	V        string `json:"v"`
	T        int64  `json:"t"`
	Priority int64  `json:"priority"`   // default: 0, the bigger value means higher priority
	Deadline int64  `json:"autoremove"` // Unix timestamp, Value will be auto removed if time.Now().Unix() > Deadline
}

type OpType string

const (
	Set OpType = "set"
	Del OpType = "del"
)

type Op struct {
	Type OpType `json:"op"`
	K    string `json:"k"`
	V    Value  `json:"v"`
}

type SyncMap struct {
	mu *sync.Mutex

	achieved         map[string]Value // key
	achieved_version int64

	op_list          *list.List
	op_list_max_size int
}

func New(op_list_max_size int) *SyncMap {
	if op_list_max_size < 1 {
		panic("op_list_max_size is too small!")
	}
	return &SyncMap{
		mu:               new(sync.Mutex),
		achieved:         make(map[string]Value),
		achieved_version: 1,
		op_list:          list.New(),
		op_list_max_size: op_list_max_size,
	}
}

func (sm *SyncMap) version() int64 {
	if sm.op_list.Len() > 0 {
		op := sm.op_list.Back().Value.(Op)
		return op.V.T
	}
	return sm.achieved_version
}

func (sm *SyncMap) next_version() int64 {
	version := time.Now().UnixNano() / 1000
	now_version := sm.version()
	if version <= now_version {
		return now_version + 1
	}
	return version
}

func (sm *SyncMap) SetWithOptions(key, value string, priority int, autoremove int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if autoremove <= 0 {
		autoremove = 60 * 60 * 1 // 1 hour
	}

	sm.op_list.PushBack(Op{
		Type: Set,
		K:    key,
		V: Value{
			V:        value,
			T:        sm.next_version(),
			Priority: int64(priority),
			Deadline: int64(autoremove) + time.Now().Unix(),
		},
	})

	sm.achieve(false)
}
func (sm *SyncMap) SetWithPriority(key, value string, priority int) {
	sm.SetWithOptions(key, value, priority, 0)
}
func (sm *SyncMap) Set(key, value string) {
	sm.SetWithOptions(key, value, 0, 0)
}

func (sm *SyncMap) AutoRemove() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now().Unix()
	for _, k := range sm.keys() {
		v := sm.get(k)
		if now > v.Deadline {
			sm.del(k, false)
		}
	}
	sm.achieve(false)
}

func (sm *SyncMap) achieve(force bool) {
	for sm.op_list.Len() > sm.op_list_max_size || (force && sm.op_list.Len() > 0) {
		op := sm.op_list.Remove(sm.op_list.Front()).(Op)
		switch op.Type {
		case Set:
			sm.achieved[op.K] = op.V
		case Del:
			delete(sm.achieved, op.K)
		default:
			panic("Unsupported operation type")
		}
		sm.achieved_version = op.V.T
	}
}

func (sm *SyncMap) del(key string, try_achieve bool) {
	sm.op_list.PushBack(Op{
		Type: Del,
		K:    key,
		V: Value{
			V: "",
			T: sm.next_version(),
		},
	})
	if try_achieve {
		sm.achieve(false)
	}
}

func (sm *SyncMap) Del(key string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.del(key, true)
}

func (sm *SyncMap) get(key string) Value {
	for e := sm.op_list.Back(); e != nil; e = e.Prev() {
		op := e.Value.(Op)
		if op.K == key {
			switch op.Type {
			case Set:
				return op.V
			case Del:
				return Value{}
			default:
				panic("Unsupported operation type")
			}
		}
	}
	return sm.achieved[key]
}

func (sm *SyncMap) keys() []string {
	keyset := make(map[string]bool)

	for k, _ := range sm.achieved {
		keyset[k] = true
	}

	for e := sm.op_list.Front(); e != nil; e = e.Next() {
		op := e.Value.(Op)
		switch op.Type {
		case Set:
			keyset[op.K] = true
		case Del:
			delete(keyset, op.K)
		default:
			panic("Unsupported operation type")
		}
	}

	ret := make([]string, 0, len(keyset))
	for k, _ := range keyset {
		ret = append(ret, k)
	}
	return ret
}

func (sm *SyncMap) Get(key string) Value {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.get(key)
}

type Patch struct {
	Achieved        map[string]Value `json:"achieved"`
	AchievedVersion int64            `json:"achieved_version"`
	Op              []Op             `json:"op"`
}

func (sm *SyncMap) Version() int64 {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.version()
}

func (sm *SyncMap) Diff(after_version int64) Patch {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	patch := Patch{
		Achieved:        make(map[string]Value),
		AchievedVersion: 0,
		Op:              make([]Op, 0, sm.op_list.Len()),
	}

	if after_version > sm.version() {
		after_version = 0
	}
	if after_version == sm.version() {
		return patch
	}

	if after_version < sm.achieved_version {
		for k, v := range sm.achieved {
			patch.Achieved[k] = v
		}
		patch.AchievedVersion = sm.achieved_version
	}

	for e := sm.op_list.Front(); e != nil; e = e.Next() {
		op := e.Value.(Op)
		if op.V.T > after_version {
			patch.Op = append(patch.Op, op)
		}

	}
	return patch
}

func (sm *SyncMap) Patch(patch Patch) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(patch.Achieved) > 0 || patch.AchievedVersion > 0 {
		sm.achieved = make(map[string]Value)
		sm.achieved_version = patch.AchievedVersion
		for k, v := range patch.Achieved {
			sm.achieved[k] = v
		}
		sm.op_list.Init()
	}

	version := sm.version()
	for _, op := range patch.Op {
		if op.V.T > version {
			sm.op_list.PushBack(op)
		}
	}
	sm.achieve(false)
}

func (sm *SyncMap) ForceAchieve() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.achieve(true)
}

func (sm *SyncMap) Size() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return len(sm.keys())
}
