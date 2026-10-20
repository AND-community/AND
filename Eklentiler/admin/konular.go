package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
)

type allPostsMsg struct {
	posts []pluginapi.PostSummary
	err   error
}
type deleteMsg struct{ err error }

func allPostsCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		posts, err := c.AllPosts()
		return allPostsMsg{posts: posts, err: err}
	}
}

func deleteCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return deleteMsg{err: c.DeletePost(postID)}
	}
}

func (m model) handleKonularKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.postIdx > 0 {
			m.postIdx--
		}
	case "down", "j":
		if m.postIdx < len(m.allPosts)-1 {
			m.postIdx++
		}
	case "r", "R":
		m.loading = true
		return m, allPostsCmd(m.client)
	case "x", "X":
		if m.pending || len(m.allPosts) == 0 {
			return m, nil
		}
		m.confirming = true
		m.notice = ""
	}
	return m, nil
}

func (m model) handleKonularConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n", "h":
		m.confirming = false
		return m, nil
	case "e", "y", "enter":
		m.confirming = false
		if m.postIdx >= len(m.allPosts) {
			return m, nil
		}
		m.pending = true
		return m, deleteCmd(m.client, m.allPosts[m.postIdx].ID)
	}
	return m, nil
}

func (m model) viewKonular() string {
	var sb strings.Builder
	if len(m.allPosts) == 0 {
		sb.WriteString(stDim.Render("Hiç konu yok.") + "\n")
		return sb.String()
	}
	for i, p := range m.allPosts {
		line := fmt.Sprintf("[%s] %s  %s", safe(p.Category), safe(p.Title), stDim.Render(safe(p.AuthorName)))
		if i == m.postIdx {
			sb.WriteString(stSel.Render(line) + "\n")
		} else {
			sb.WriteString(stNormal.Render(line) + "\n")
		}
	}
	if m.confirming && m.postIdx < len(m.allPosts) {
		title := safe(m.allPosts[m.postIdx].Title)
		sb.WriteString("\n" + stErr.Render(fmt.Sprintf("Sil: %s — emin misiniz?", title)) + "\n")
	}
	return sb.String()
}
