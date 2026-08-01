// Package app is the top-level owner (AppState): it manages tabs,
// global hotkeys, and switching between active tabs.
//
// The pure tab/hotkey state machine lives in state.go and is unit-tested; run.go
// holds the Ebitengine main loop and input plumbing, which are verified by
// running the app.
package app
