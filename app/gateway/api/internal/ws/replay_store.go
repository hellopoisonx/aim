// Package ws — pending/replay 队列。
//
// ReplayStore 在连接维度保存可重放的服务端推送帧，用于客户端心跳上报
// HeartbeatPayload.last_seq 时识别漏帧并补发。L1 复用项目共享缓存组件
// （go-zero collection.Cache），避免手写无界 map。
package ws

import (
	"sync"
	"time"

	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/collection"
	"google.golang.org/protobuf/proto"
)

const (
	// defaultReplayCapacity 每连接保留的最大可重放帧数。
	defaultReplayCapacity = 128
	// defaultReplayTTL 单条 pending 帧的存活时间。
	defaultReplayTTL = 5 * time.Minute
	// defaultReplayL1Limit L1 缓存可容纳的连接数量上限。
	defaultReplayL1Limit = 4096
)

// replayableFrameTypes 列出允许进入 pending 队列的服务端推送帧白名单。
// 与 docs/ws.md / aim-gateway-domain skill 中的 pending 白名单保持一致。
var replayableFrameTypes = map[wspb.FrameType]struct{}{
	wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE:            {},
	wspb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION:       {},
	wspb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION: {},
	wspb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT:       {},
}

// IsReplayable 判断帧类型是否进入 pending 队列。
func IsReplayable(t wspb.FrameType) bool {
	_, ok := replayableFrameTypes[t]
	return ok
}

// ReplayFrame 是 pending 队列里的一条记录。
type ReplayFrame struct {
	Frame   *wspb.WsFrame
	StoreAt time.Time
}

// ReplayStore 在连接维度维护 pending 队列。
type ReplayStore struct {
	l1       *collection.Cache
	capacity int
	ttl      time.Duration

	// mu 保护对每个 identity 队列的读改写，避免 collection.Cache 的 Get/Set
	// 之间发生并发竞争。
	mu sync.Mutex
}

// NewReplayStore 构造默认 L1 容量与 TTL 的 ReplayStore。
func NewReplayStore() (*ReplayStore, error) {
	return NewReplayStoreWithOptions(defaultReplayCapacity, defaultReplayTTL, defaultReplayL1Limit)
}

// NewReplayStoreWithOptions 提供自定义容量/TTL；用于测试或后续配置注入。
func NewReplayStoreWithOptions(capacity int, ttl time.Duration, l1Limit int) (*ReplayStore, error) {
	if capacity <= 0 {
		capacity = defaultReplayCapacity
	}
	if ttl <= 0 {
		ttl = defaultReplayTTL
	}
	if l1Limit <= 0 {
		l1Limit = defaultReplayL1Limit
	}

	l1, err := collection.NewCache(ttl, collection.WithLimit(l1Limit), collection.WithName(sharedcache.NameWsReplay))
	if err != nil {
		return nil, err
	}

	return &ReplayStore{l1: l1, capacity: capacity, ttl: ttl}, nil
}

func (s *ReplayStore) key(identity Identity) string {
	return sharedcache.WsReplayKey(identity.UserID, identity.DeviceID)
}

// loadLocked 读取当前 identity 的 pending 切片，调用方必须持有 s.mu。
func (s *ReplayStore) loadLocked(identity Identity) []ReplayFrame {
	val, ok := s.l1.Get(s.key(identity))
	if !ok || val == nil {
		return nil
	}
	frames, ok := val.([]ReplayFrame)
	if !ok {
		// 类型不匹配则视为脏数据，主动清理。
		s.l1.Del(s.key(identity))
		return nil
	}
	return frames
}

// storeLocked 把切片写回 L1；空切片直接删除条目。
func (s *ReplayStore) storeLocked(identity Identity, frames []ReplayFrame) {
	if len(frames) == 0 {
		s.l1.Del(s.key(identity))
		return
	}
	s.l1.Set(s.key(identity), frames)
}

// pruneLocked 删除过期帧并裁剪到容量上限，要求 frames 按 seq 升序。
func (s *ReplayStore) pruneLocked(frames []ReplayFrame, now time.Time) []ReplayFrame {
	if len(frames) == 0 {
		return frames
	}

	// 丢弃过期帧（slice 前缀通常先过期，但保守地全量扫描）。
	cutoff := now.Add(-s.ttl)
	keep := frames[:0]
	for _, f := range frames {
		if f.StoreAt.Before(cutoff) {
			continue
		}
		keep = append(keep, f)
	}

	// 容量上限：保留最新的 capacity 帧。
	if len(keep) > s.capacity {
		drop := len(keep) - s.capacity
		keep = keep[drop:]
	}

	// 复制一份避免共享底层数组，便于 GC 释放被裁掉的帧。
	out := make([]ReplayFrame, len(keep))
	copy(out, keep)
	return out
}

// Append 把一条已分配 seq 的可重放帧加入 pending 队列。frame 会被克隆，
// 调用方在 Append 之后修改原 frame 不影响 store。
func (s *ReplayStore) Append(identity Identity, frame *wspb.WsFrame) {
	if s == nil || frame == nil {
		return
	}
	if !IsReplayable(frame.GetType()) {
		return
	}
	if frame.GetSeq() <= 0 {
		// seq 必须为正数，未分配的帧不应进入 pending。
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	frames := s.loadLocked(identity)
	frames = append(frames, ReplayFrame{
		Frame:   proto.Clone(frame).(*wspb.WsFrame),
		StoreAt: time.Now(),
	})
	frames = s.pruneLocked(frames, time.Now())
	s.storeLocked(identity, frames)
}

// Ack 删除所有 seq <= ackSeq 的 pending 帧，并执行 TTL/容量裁剪。
func (s *ReplayStore) Ack(identity Identity, ackSeq int64) {
	if s == nil || ackSeq <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	frames := s.loadLocked(identity)
	if len(frames) == 0 {
		return
	}

	keep := frames[:0]
	for _, f := range frames {
		if f.Frame.GetSeq() <= ackSeq {
			continue
		}
		keep = append(keep, f)
	}
	keep = s.pruneLocked(keep, time.Now())
	s.storeLocked(identity, keep)
}

// PendingAfter 返回 seq > lastSeq 的可重放帧（按 seq 升序）。返回的帧是
// 克隆，调用方可以安全发送/修改。同时执行一次 TTL/容量裁剪。
func (s *ReplayStore) PendingAfter(identity Identity, lastSeq int64) []*wspb.WsFrame {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	frames := s.loadLocked(identity)
	frames = s.pruneLocked(frames, time.Now())
	s.storeLocked(identity, frames)

	if len(frames) == 0 {
		return nil
	}

	out := make([]*wspb.WsFrame, 0, len(frames))
	for _, f := range frames {
		if f.Frame.GetSeq() <= lastSeq {
			continue
		}
		out = append(out, proto.Clone(f.Frame).(*wspb.WsFrame))
	}
	return out
}

// Delete 移除某连接的全部 pending 帧；连接断开/重新注册时调用。
func (s *ReplayStore) Delete(identity Identity) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.l1.Del(s.key(identity))
}

// Len 返回当前 identity 的 pending 数量（不含裁剪）。仅用于测试。
func (s *ReplayStore) Len(identity Identity) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.loadLocked(identity))
}
