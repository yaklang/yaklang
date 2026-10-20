package ssa

import (
	"slices"

	"github.com/yaklang/yaklang/common/utils"
)

// has/get user and value
func (n *anValue) HasUsers() bool {
	return len(n.snapshotUserIDs()) != 0
}
func (n *anValue) GetUsers() Users {
	ids := n.snapshotUserIDs()
	if len(ids) == 0 {
		return nil
	}
	return n.GetUsersByIDs(ids)
}

// Published prefixes are immutable: append writes beyond their length and
// removal allocates a replacement. Readers can resolve IDs outside the lock
// without copying the full use list on every traversal.
func (n *anValue) snapshotUserIDs() []int64 {
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	return n.userList
}

// addUserSetThreshold is the userList length at which AddUser switches from
// a linear duplicate scan to the userSet map.
const addUserSetThreshold = 16

// for Value
func (n *anValue) AddUser(u User) {
	id := u.GetId()
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	if n.userSet != nil {
		if _, ok := n.userSet[id]; ok {
			return
		}
		n.userSet[id] = struct{}{}
		n.userList = append(n.userList, id)
		n.markPersistenceDirty()
		return
	}
	if index := slices.Index(n.userList, id); index == -1 {
		n.userList = append(n.userList, id)
		n.markPersistenceDirty()
		if len(n.userList) >= addUserSetThreshold {
			n.userSet = make(map[int64]struct{}, len(n.userList))
			for _, uid := range n.userList {
				n.userSet[uid] = struct{}{}
			}
		}
	}
}

func (n *anValue) RemoveUser(u User) {
	id := u.GetId()
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	if len(n.userList) == 0 || !slices.Contains(n.userList, id) {
		return
	}
	n.userList = utils.RemoveSliceItem(append([]int64(nil), n.userList...), id)
	if n.userSet != nil {
		delete(n.userSet, id)
	}
	n.markPersistenceDirty()
}
