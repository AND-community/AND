// Package tui implements AND's keyboard-only terminal interface using
// Charmbracelet's bubbletea (program/model/update loop), bubbles
// (textinput/viewport components), and lipgloss (styling).
//
// There are two entry points, run as separate bubbletea programs in
// sequence:
//
//   - Login (login.go) — the unlock screen for a returning identity, or
//     the registration form (display name + passphrase, then a one-time
//     mnemonic confirmation) for a brand new one.
//   - Run (app.go) — the main app shell once an identity is unlocked: a
//     menu, a forum browser (placeholder until internal/forum has a real
//     post/thread model), and live chat over internal/network's
//     ChatTopic.
//
// cmd/and wires the two together: Login first, then bring up the libp2p
// node/pubsub topics, then Run.
package tui
