package telegram

import "sync"

type UserStatus struct {
	store map[int]int
	mu    sync.RWMutex
}

func (u *UserStatus) Set(userID, status int) {
	if u.store == nil {
		u.store = make(map[int]int)
	}
	u.mu.Lock()
	u.store[userID] = status
	u.mu.Unlock()
}

func (u *UserStatus) Get(userID int) int {
	if u.store == nil {
		return UStatusUndefined
	}
	u.mu.RLock()
	status := u.store[userID]
	u.mu.RUnlock()
	return status
}

const (
	UStatusUndefined = iota

	UStatusCreateGroupSetPassword

	UStatusJoinGroupCheckGroup
	UStatusJoinGroupCheckPassword

	UStatusCreateItemSetURL
	UStatusCreateItemSetName
)
