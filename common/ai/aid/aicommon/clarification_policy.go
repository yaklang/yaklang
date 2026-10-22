package aicommon

import "fmt"

// SetClarificationPolicy scopes questions to a runtime role/phase. It is not
// inherited by child runtimes; workers explicitly disable user interaction.
func (c *Config) SetClarificationPolicy(policy func() (allowed, blocking bool, limit int64)) {
	c.clarificationMu.Lock()
	c.clarificationPolicy = policy
	c.clarificationMu.Unlock()
}

func (c *Config) clarificationPolicyValues() (bool, bool, int64) {
	c.clarificationMu.Lock()
	if c.m != nil {
		c.m.Lock()
	}
	allowed, blocking := c.AllowRequireForUserInteract, !c.EnableGoalMode && !c.disableBlockingClarification
	policy, explicit, limit := c.clarificationPolicy, c.userInteractLimitExplicit, c.PerTaskUserInteractiveLimitedTimes
	c.clarificationMu.Unlock()
	if c.m != nil {
		c.m.Unlock()
	}
	if limit <= 0 {
		limit = 3
	}
	if policy != nil {
		roleAllowed, roleBlocking, roleLimit := policy()
		allowed, blocking = allowed && roleAllowed, blocking && roleBlocking
		if roleLimit > 0 && (!explicit || roleLimit < limit) {
			limit = roleLimit
		}
	}
	return allowed, blocking, limit
}

func (c *Config) ClarificationUsage(taskID string) (count, limit int64) {
	_, _, limit = c.clarificationPolicyValues()
	c.clarificationMu.Lock()
	defer c.clarificationMu.Unlock()
	return c.clarificationCounts[taskID], limit
}

func (c *Config) checkClarificationLocked(taskID string, blocking, allowed, canBlock bool, limit int64) error {
	if !allowed {
		return fmt.Errorf("user clarification is disabled for this runtime or planning phase; report missing information to the parent if this is a worker")
	}
	if blocking && !canBlock {
		return fmt.Errorf("blocking clarification is disabled here; use ask_for_clarification_async and continue independent work")
	}
	if c.clarificationCounts[taskID] >= limit {
		return fmt.Errorf("user clarification limit (%d calls per task, shared by synchronous and asynchronous questions) reached", limit)
	}
	return nil
}

func (c *Config) CheckClarificationAllowed(taskID string, blocking bool) error {
	allowed, canBlock, limit := c.clarificationPolicyValues()
	c.clarificationMu.Lock()
	defer c.clarificationMu.Unlock()
	return c.checkClarificationLocked(taskID, blocking, allowed, canBlock, limit)
}

// AcquireClarification consumes one call (a batch of up to three questions).
// Admission is atomic, so parallel callers cannot bypass the shared task budget.
func (c *Config) AcquireClarification(taskID string, blocking bool) error {
	allowed, canBlock, limit := c.clarificationPolicyValues()
	c.clarificationMu.Lock()
	defer c.clarificationMu.Unlock()
	if err := c.checkClarificationLocked(taskID, blocking, allowed, canBlock, limit); err != nil {
		return err
	}
	if c.clarificationCounts == nil {
		c.clarificationCounts = make(map[string]int64)
	}
	c.clarificationCounts[taskID]++
	return nil
}

// ReleaseClarification refunds a request that failed before it was emitted.
func (c *Config) ReleaseClarification(taskID string) {
	c.clarificationMu.Lock()
	defer c.clarificationMu.Unlock()
	if c.clarificationCounts[taskID] > 0 {
		c.clarificationCounts[taskID]--
	}
}

func (c *Config) PlanClarificationSettings() (bool, int64) {
	if c.m != nil {
		c.m.Lock()
		defer c.m.Unlock()
	}
	limit := c.PlanUserInteractMaxCount
	if limit <= 0 {
		limit = 3
	}
	return c.AllowPlanUserInteract, limit
}
