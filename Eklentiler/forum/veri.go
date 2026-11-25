package main

// veri.go — forum eklentisinin AND'la konuşması (pluginapi) ve konu
// listesinin sıralanıp süzülmesi. Konular GET /api/v1/forum/posts ile toplu,
// bir konunun ayrıntısı ve yanıtları GET /api/v1/forum/post ile gelir;
// değişiklikler GET /api/v1/forum/changes uzun yoklamasıyla izlenir.

import (
	"errors"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/pluginapi"
)

// istemci, eklentinin kullandığı AND API'sidir (*pluginapi.Client; testte
// sahtesi).
type istemci interface {
	ForumPosts() ([]pluginapi.PostSummary, error)
	GetPost(id string) (*pluginapi.PostDetail, error)
	CreatePost(category, title, body string) error
	CreateReply(postID, body string) error
	DeleteOwnPost(postID string) error
	Profile(key string) (*pluginapi.ProfileInfo, error)
	ForumChanges(since uint64) (pluginapi.ForumChanges, error)
}

type (
	konularMsg struct {
		konular []pluginapi.PostSummary
		err     error
	}
	konuMsg struct {
		id    string
		detay *pluginapi.PostDetail
		err   error
	}
	degisimMsg struct {
		d   pluginapi.ForumChanges
		err error
	}
	degisimTekrarMsg   struct{}
	yanitGonderildiMsg struct {
		konuID string
		err    error
	}
	silindiMsg struct{ err error }
	profilMsg  struct {
		p   *pluginapi.ProfileInfo
		err error
	}
	bildirimTemizleMsg struct{}
)

func konularYukle(c istemci) tea.Cmd {
	return func() tea.Msg {
		k, err := c.ForumPosts()
		return konularMsg{konular: k, err: err}
	}
}

func konuYukle(c istemci, id string) tea.Cmd {
	return func() tea.Msg {
		d, err := c.GetPost(id)
		return konuMsg{id: id, detay: d, err: err}
	}
}

// degisimBekle, since'ten sonraki ilk forum değişikliğini bekler (AND en çok
// ~25 sn tutar; değişiklik yoksa aynı sürüm döner).
func degisimBekle(c istemci, since uint64) tea.Cmd {
	return func() tea.Msg {
		d, err := c.ForumChanges(since)
		return degisimMsg{d: d, err: err}
	}
}

// yenidenDene, geçici bir yoklama hatasından sonra 5 sn bekler.
func yenidenDene() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return degisimTekrarMsg{} })
}

func bildirimTemizle() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return bildirimTemizleMsg{} })
}

// ucYok, isteğin eski bir AND'da olmayan bir uca gittiğidir (404 ya da 501):
// o zaman canlı güncelleme kapanır, liste elle yenilenir.
func ucYok(err error) bool {
	var ae *pluginapi.APIError
	return errors.As(err, &ae) && (ae.Status == 404 || ae.Status == 501)
}

// sonEtkinlik, konunun son yanıtının (yoksa kendisinin) zamanıdır; eski bir
// AND son etkinliği göndermezse oluşturma zamanı kullanılır.
func sonEtkinlik(p pluginapi.PostSummary) time.Time {
	if !p.LastActivity.IsZero() {
		return p.LastActivity
	}
	return p.CreatedAt
}

// siralaKonular, konuları son etkinliğe göre (yeniden eskiye) sıralar.
func siralaKonular(k []pluginapi.PostSummary) []pluginapi.PostSummary {
	k = slices.Clone(k)
	slices.SortStableFunc(k, func(a, b pluginapi.PostSummary) int { return sonEtkinlik(b).Compare(sonEtkinlik(a)) })
	return k
}

// kategoriKonulari, seçili kategorinin konularıdır (kategori boşsa hepsi).
func (m model) kategoriKonulari() []pluginapi.PostSummary {
	if m.kategori == "" {
		return m.konular
	}
	var out []pluginapi.PostSummary
	for _, k := range m.konular {
		if k.Category == m.kategori {
			out = append(out, k)
		}
	}
	return out
}

// kategoriSayilari, her kategorideki konu sayısıdır.
func (m model) kategoriSayilari() map[string]int {
	say := map[string]int{}
	for _, k := range m.konular {
		say[k.Category]++
	}
	return say
}
