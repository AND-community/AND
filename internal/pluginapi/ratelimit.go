package pluginapi

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/lucian95511/and/internal/timewin"
)

// localActionLimiter, TEK bir yerel eklenti uç noktası KATEGORİSİ (ör.
// "forum gönderisi oluşturma") için paylaşılan bir kayan-pencere hız
// sınırlayıcıdır.
//
// AND'ın kendi TUI'si insan hızında çalıştığından bu sınıra hiçbir zaman
// takılmaz — bu sınır yalnızca pluginapi ÜZERİNDEN (bir eklenti, ya da
// AND_ENABLE_EXTERNAL_API=1 iken aynı makinedeki herhangi bir yerel süreç)
// gelen çağrılar için vardır. internal/forum/internal/chatlog/internal/site
// kasıtlı olarak "kendi imzaladığın hiçbir şey reddedilmez" ilkesiyle
// çalışır (bkz. o paketlerin persist()'lerinin doc yorumu — bu, AĞDAN
// gelen mesajlara uygulanan Sybil/hız sınırlarının, sahibinin KENDİ dürüst
// düzenlemelerini engellememesi için kasıtlıdır). Bu katman olmadan hatalı
// ya da kötü niyetli bir eklenti binary'si, bu düğümün kendi GEÇERLİ
// imzasıyla ağı hızlıca gerçek gönderi/mesaj/sayfa akışıyla doldurabilir —
// kimseyi taklit etmez ama düğümün kendi itibarını/bant genişliğini
// tüketir ve ağdaki diğer düğümlerin depolarını şişirir.
type localActionLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   []time.Time
}

func newLocalActionLimiter(max int, window time.Duration) *localActionLimiter {
	return &localActionLimiter{max: max, window: window}
}

// allow, bir eyleme şimdi izin verilip verilmediğini bildirir — izin
// verilirse bunu da pencereye ekler.
func (l *localActionLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.window)
	l.hits = timewin.KeepAfter(l.hits, cutoff)
	if len(l.hits) >= l.max {
		return false
	}
	l.hits = append(l.hits, now)
	return true
}

// Kategori başına sınırlar — doğal insan temposuna göre kabaca ölçeklenir,
// ama TEK bir insan eylemi ARKA PLANDA BİRDEN FAZLA çağrıya yayılabilen
// meşru toplu işlemleri kırmayacak kadar da cömerttir: DeltaX'in "Gönder
// (push) → Çevrimiçi herkese" ve grup roster/davet yayını (bkz.
// Eklentiler/DeltaX/sync.go'daki "for _, u := range g.Uyeler" döngüleri)
// TEK bir kullanıcı tıklamasıyla düzinelerce dm/send ya da file/send
// çağrısı üretebilir — dmLimiter/fileLimiter bu yüzden contentLimiter'dan
// belirgin şekilde daha yüksektir.
// contentActionLimit forum'a VE site'a AYNI DEĞERLE ama AYRI (Server'daki
// forumLimiter/siteLimiter) sınırlayıcı ÖRNEKLERİYLE uygulanır — hatalı ya
// da kötü niyetli TEK bir eklentinin (ör. forum'u kötüye kullanan bir
// eklenti) kendi bütçesini tüketmesi, site sayfası yayınlamak isteyen
// İLGİSİZ bir eklentiyi hiç etkilememelidir.
const (
	contentActionLimit  = 20  // forum post/yanıt VE site yayın/silme (ayrı ayrı) — dakikada (toplu bir kullanım deseni yok)
	chatActionLimit     = 60  // genel sohbet gönderimi — dakikada
	dmActionLimit       = 120 // DM gönderimi — dakikada (DeltaX grup roster/davet yayını, tek eylemde çok alıcı)
	fileActionLimit     = 30  // dosya gönderimi — dakikada (DeltaX "herkese gönder", tek eylemde çok alıcı)
	localActionsWindow  = time.Minute
	rateLimitErrMessage = "çok hızlı gönderim — birkaç saniye bekleyip tekrar deneyin"
)

func writeTooManyRequests(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(errResp{Error: msg}) //nolint:errcheck
}
