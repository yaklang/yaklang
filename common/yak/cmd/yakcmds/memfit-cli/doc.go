// Package memfitcli owns the public yak memfit command and its hidden worker
// command. The interactive frontend communicates only with a separately
// spawned copy of the yak executable; the AI engine never runs in the TUI
// process.
package memfitcli
