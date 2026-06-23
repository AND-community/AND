package tui

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
