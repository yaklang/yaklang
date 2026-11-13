package aicommon

import "context"

// attachedBrowserTestLoop intentionally implements only the neutral plumbing
// needed by browser attachment routing tests without importing reactloops back
// into aicommon.
type attachedBrowserTestLoop struct {
	config  AICallerConfigIf
	invoker AIInvokeRuntime
}

func (*attachedBrowserTestLoop) Execute(string, context.Context, string) error { return nil }
func (*attachedBrowserTestLoop) ExecuteWithExistedTask(AIStatefulTask) error   { return nil }
func (*attachedBrowserTestLoop) GetCurrentTask() AIStatefulTask                { return nil }
func (*attachedBrowserTestLoop) SetCurrentTask(AIStatefulTask)                 {}
func (l *attachedBrowserTestLoop) GetInvoker() AIInvokeRuntime                 { return l.invoker }
func (*attachedBrowserTestLoop) GetEmitter() *Emitter                          { return nil }
func (l *attachedBrowserTestLoop) GetConfig() AICallerConfigIf                 { return l.config }
func (*attachedBrowserTestLoop) GetMemoryTriage() MemoryTriage                 { return nil }
func (*attachedBrowserTestLoop) Set(string, any)                               {}
func (*attachedBrowserTestLoop) Get(string) string                             { return "" }
func (*attachedBrowserTestLoop) GetVariable(string) any                        { return nil }
func (*attachedBrowserTestLoop) GetStringSlice(string) []string                { return nil }
func (*attachedBrowserTestLoop) GetInt(string) int                             { return 0 }
func (*attachedBrowserTestLoop) RemoveAction(string)                           {}
func (*attachedBrowserTestLoop) GetAllActionNames() []string                   { return nil }
func (*attachedBrowserTestLoop) NoActions() bool                               { return false }
func (*attachedBrowserTestLoop) PushMemory(*SearchMemoryResult)                {}
func (*attachedBrowserTestLoop) GetCurrentMemoriesContent() string             { return "" }
func (*attachedBrowserTestLoop) DisallowAskForClarification()                  {}
func (*attachedBrowserTestLoop) GetTimelineDiff() (string, error)              { return "", nil }
