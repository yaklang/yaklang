package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueConcurrentUsersAndStableSnapshot(t *testing.T) {
	value := NewValue()
	users := make([]*Call, 512)
	for i := range users {
		users[i] = &Call{anValue: NewValue()}
		users[i].SetId(int64(i + 1))
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, user := range users {
				value.AddUser(user)
				_ = value.HasUsers()
			}
		}()
	}
	wg.Wait()
	before := value.snapshotUserIDs()
	require.Len(t, before, len(users))
	value.RemoveUser(users[0])
	require.Len(t, value.snapshotUserIDs(), len(users)-1)
	require.Contains(t, before, int64(1))
	value.AddUser(users[0])
	require.Len(t, value.snapshotUserIDs(), len(users))
}
