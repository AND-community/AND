package tui

import "github.com/lucian95511/and/internal/githubverify"

// backMsg is sent by screens to navigate back to the main menu.
type backMsg struct{}

// openExternalPluginMsg asks the app to launch an external plugin subprocess.
// The env field carries KEY=VALUE strings appended to the subprocess environment.
type openExternalPluginMsg struct {
	name string
	env  []string
}

// pluginExitMsg is delivered when a plugin subprocess exits.
type pluginExitMsg struct {
	name string
	err  error
}

// profileSavedMsg is delivered after an attempt to publish the local
// identity's full profile (see appModel.savePublishedProfile).
type profileSavedMsg struct{ err error }

// githubDeviceCodeMsg is delivered after githubverify.RequestDeviceCode
// returns — success carries the code to display; failure aborts the flow
// (see appModel.startGitHubLink and its Update handler).
type githubDeviceCodeMsg struct {
	dc  *githubverify.DeviceCode
	err error
}

// githubLinkedMsg is delivered once the background poll-then-publish
// sequence started by the githubDeviceCodeMsg handler finishes: success,
// denial, expiry, or cancellation (see appModel.handleGitHubLinkKey).
type githubLinkedMsg struct {
	username string
	err      error
}

// chatHistoryChangedMsg is delivered when chatlog.Store.SyncWithPeer
// backfills chat messages from a peer (see chatlog.Store.HistoryChanged) —
// the Update handler reloads the full history from Store.All() rather than
// appending, since backfilled messages can be older than anything already
// shown.
type chatHistoryChangedMsg struct{}
