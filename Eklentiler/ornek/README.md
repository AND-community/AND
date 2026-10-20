# Eklenti Geliştirme Rehberi

AND eklentileri bağımsız Go binary'leridir. Her eklentinin bir `plugin.json` manifest dosyası ve bir binary'si vardır. AND, manifest'i **binary çalıştırmadan** okur (XenForo'nun `addon.json` yapısına benzer), ardından kullanıcı seçtiğinde eklentiyi HTTP IPC üzerinden başlatır.

---

## İçindekiler

- [Genel Bakış](#genel-bakış)
- [Hızlı Başlangıç](#hızlı-başlangıç)
- [plugin.json — Statik Manifest](#pluginjson--statik-manifest)
- [main.go Şablonu](#maingo-şablonu)
- [AND_API_ADDR — HTTP IPC API](#and_api_addr--http-ipc-api)
- [Tüm API Endpoint'leri](#tüm-api-endpointleri)
- [Bubbletea TUI ile Tam Örnek](#bubbletea-tui-ile-tam-örnek)
- [Görünüm — AND'ın Görsel Dili](#görünüm--andın-görsel-dili)
- [Gizli Eklenti — Forumdan Açılan](#gizli-eklenti--forumdan-açılan)
- [AND_DATA_DIR — Yerel Depolama](#and_data_dir--yerel-depolama)
- [Keşif Kuralları](#keşif-kuralları)
- [Açma / Kapama](#açma--kapama)
- [Test Etme](#test-etme)

---

## Genel Bakış

```
AND başlar
  └─ kendi dizinindeki and-plugin-* dosyalarını tarar
       └─ her binary için and-plugin-<name>.json sidecar'ı kontrol eder (hızlı, kod çalışmaz)
            └─ sidecar yoksa --manifest ile binary çalıştırır (yavaş, fallback)
       └─ plugins_state.json'dan açık/kapalı durumunu yükler

Kullanıcı menüden eklenti seçer (kapalı eklentiler gri gösterilir, enter çalışmaz)
  └─ AND, binary'yi şu env ile başlatır:
       AND_API_ADDR=127.0.0.1:<port>   ← AND'ın HTTP API adresi
       AND_DATA_DIR=/path/to/data      ← veri dizini
  └─ Eklenti kendi Bubbletea TUI'sını çalıştırır
  └─ Eklenti çıkınca AND menüye döner
```

---

## Hızlı Başlangıç

### 1. Dizin yap, manifest ve kaynak yaz

```bash
mkdir -p Eklentiler/meklentim
```

### 2. plugin.json oluştur

Her eklentinin kendi dizininde bir `plugin.json` olması **zorunludur**:

```json
{
  "name":        "meklentim",
  "label":       "Benim Eklentim",
  "version":     "1.0.0",
  "description": "Ne yaptığını anlat",
  "author":      "Adın Soyadın",
  "requires":    []
}
```

### 3. Derle ve AND dizinine koy

```bash
# Linux / macOS
go build -o and-plugin-meklentim ./Eklentiler/meklentim

# Windows
go build -o and-plugin-meklentim.exe ./Eklentiler/meklentim

# Sidecar JSON'u binary'nin yanına kopyala
cp Eklentiler/meklentim/plugin.json and-plugin-meklentim.json
```

Hedef dizin yapısı:

```
/hedef/dizin/
  and[.exe]
  and-plugin-meklentim[.exe]   ← binary
  and-plugin-meklentim.json    ← sidecar manifest (AND bunu okur, hız için)
```

AND yeniden başlattığında eklenti otomatik keşfedilir.

---

## plugin.json — Statik Manifest

XenForo'nun `addon.json` dosyasına karşılık gelir. AND bu dosyayı okuyarak binary'i çalıştırmadan eklenti bilgisini alır.

```json
{
  "name":        "meklentim",
  "label":       "Benim Eklentim",
  "version":     "1.0.0",
  "description": "Ne yaptığını anlat",
  "author":      "Adın Soyadın",
  "requires":    []
}
```

| Alan | Zorunlu | Açıklama |
|------|---------|----------|
| `name` | Evet | Binary dosya adındaki slug ile uyuşmalı (`and-plugin-<name>`) |
| `label` | Hayır | Ana menüdeki görünen ad; `""` → menüde görünmez (gizli eklenti) |
| `version` | Hayır | Sürüm bilgisi (menü detayında görünür) |
| `description` | Hayır | Menü detayında görünen kısa açıklama |
| `author` | Hayır | Geliştirici adı |
| `requires` | Hayır | Gelecek için; şu an kullanılmıyor |

AND, `name` alanı boş olan veya `plugin.json` bulunamayıp `--manifest` çalıştırılamayan binary'leri sessizce atlar.

---

## main.go Şablonu

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/lucian95511/and/internal/pluginapi"
)

// manifest, --manifest flag'i için fallback kaynaktır.
// plugin.json sidecar dosyası varsa AND onu kullanır (bu kod çalışmaz).
var manifest = pluginapi.Manifest{
    Name:        "meklentim",
    Label:       "Benim Eklentim",
    Version:     "1.0.0",
    Description: "Ne yaptığını anlat",
    Author:      "Adın Soyadın",
}

func main() {
    // --manifest: sidecar JSON yoksa AND bu fallback'i kullanır.
    if len(os.Args) > 1 && os.Args[1] == "--manifest" {
        data, _ := json.Marshal(manifest)
        fmt.Println(string(data))
        return
    }

    client, err := pluginapi.NewClientFromEnv()
    if err != nil {
        fmt.Fprintln(os.Stderr, "eklenti:", err)
        os.Exit(1)
    }

    p := tea.NewProgram(newModel(client), tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Fprintln(os.Stderr, "eklenti:", err)
        os.Exit(1)
    }
}
```

---

## AND_API_ADDR — HTTP IPC API

`pluginapi.NewClientFromEnv()` çağrısı `AND_API_ADDR` ortam değişkenini okur ve bir HTTP istemcisi döner.

```go
client, err := pluginapi.NewClientFromEnv()
if err != nil {
    // AND_API_ADDR tanımlı değil — binary AND dışından çalıştırılmış
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
```

### Kimlik sorgulama

```go
id, err := client.Identity()
// id.Name   → kullanıcı takma adı
// id.PubKey → Ed25519 public key (hex)
// id.PeerID → libp2p Peer ID
```

### Rol sorgulama

```go
role, err := client.Role()
// role.IsFounder   → kurucu mu?
// role.IsModerator → sertifikalı moderatör mü?
```

### Tüm konuları listele (moderatör / kurucu)

```go
posts, err := client.AllPosts()
for _, p := range posts {
    // p.ID, p.Category, p.Title, p.AuthorName, p.AuthorKey
}
```

### Konu detayı görüntüle / sil

```go
detay, err := client.GetPost("post-id-buraya")
// detay.Body, detay.CreatedAt, detay.Replies

err = client.DeletePost("post-id-buraya") // moderatör/kurucu yetkisi ister
```

### Yeni konu oluştur

```go
err = client.CreatePost("genel", "Başlık", "İçerik")
// Forumda onay sistemi yok — konu hemen ağa yayılır ve 5 gün sonra
// otomatik olarak silinir (bkz. README.md#moderasyon).
```

### Özel mesaj gönder / al

```go
err = client.SendDM("12D3KooWHedefPeerID", "Merhaba!")

msgs, err := client.PollDM()
// Mesaj gelirse msgs[0].From, msgs[0].Text, msgs[0].ReceivedAt
// 5 saniye içinde mesaj yoksa msgs boş döner

// Bu oturumun tüm özel mesajları (gelen + giden, eskiden yeniye).
// PollDM yalnızca o an dinlenirken geleni verir; geçmiş, eklenti
// kapalıyken gelenleri de içerir. Eski bir AND'da uç nokta yoktur:
kayitlar, err := client.DMHistory()
if pluginapi.IsNotFound(err) {
	// PollDM ile canlı dinlemeye geç
}

// Özel mesaj gönderilebilecek eşler: yalnızca AND düğümleri (AND == true).
esler, err := client.Peers()
```

---

## Tüm API Endpoint'leri

| Metot | Yol | Açıklama |
|-------|-----|----------|
| `GET` | `/api/v1/identity` | Kullanıcı kimliği |
| `GET` | `/api/v1/role` | Kurucu / moderatör rolü |
| `GET` | `/api/v1/forum/all-posts` | Ağdaki tüm konular (moderatör/kurucu) |
| `GET` | `/api/v1/forum/post?id=...` | Konu detayı (gövde + yanıtlar) |
| `POST` | `/api/v1/forum/delete` | Konu sil `{"post_id":"..."}` (moderatör/kurucu) |
| `POST` | `/api/v1/forum/post` | Yeni konu oluştur |
| `POST` | `/api/v1/dm/send` | DM gönder `{"peer_id":"...","message":"..."}` |
| `GET` | `/api/v1/dm/poll` | Gelen DM bekle (5 sn. long poll) |
| `GET` | `/api/v1/dm/history` | Bu oturumun gelen ve giden DM'leri (en fazla 500, yalnızca bellekte) |
| `GET` | `/api/v1/peers` | Bağlı eşler; `"and": true` olanlar AND düğümüdür (DM/dosya gönderilebilir) |

Tüm hata yanıtları `{"error":"açıklama"}` formatında JSON döner (durum kodu
hataya göre değişir: geçersiz istek 400, yetki yok 403, hız sınırı 429,
beklenmeyen sunucu hatası 500 — eklentiniz kodu değil `error` alanını
okumalı).

> **Hız sınırı:** İçerik yayınlayan/gönderen uç noktalar (forum post/yanıt,
> site yayın/silme, sohbet/DM/dosya gönderimi) yerel olarak hız
> sınırlıdır — bkz. `internal/pluginapi/ratelimit.go`. Bu, AND'ın kendi
> TUI'sini etkilemez; yalnızca eklentiler (ya da `AND_ENABLE_EXTERNAL_API=1`
> iken harici bir süreç) bu uç noktaları dakikada çok sayıda çağırırsa
> devreye girer ve `429 Too Many Requests` döner. Sınırlar insan temposunun
> çok üzerinde tutulur (ör. DM/dosya gönderimi dakikada 120/30 — DeltaX'in
> "herkese gönder" gibi tek eylemde çok alıcıya yayılan işlemlerini
> kırmayacak kadar cömert) — normal kullanımda asla görmezsiniz.

---

## Bubbletea TUI ile Tam Örnek

```go
type model struct {
    client *pluginapi.Client
    lines  []string
    input  textinput.Model
}

func newModel(c *pluginapi.Client) model {
    ti := textinput.New()
    ti.Placeholder = "mesaj..."
    ti.Focus()
    return model{client: c, input: ti}
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "esc":
            return m, tea.Quit
        case "enter":
            text := strings.TrimSpace(m.input.Value())
            if text != "" {
                _ = m.client.CreatePost("genel", text, "...", false)
                m.lines = append(m.lines, "Gönderildi: "+text)
                m.input.SetValue("")
            }
        }
    }
    var cmd tea.Cmd
    m.input, cmd = m.input.Update(msg)
    return m, cmd
}

func (m model) View() string {
    return strings.Join(m.lines, "\n") + "\n\n" + m.input.View() +
        "\n\nctrl+c çıkış"
}
```

---

## Görünüm — AND'ın Görsel Dili

Eklentin AND'ın bir ekranı gibi görünsün diye renkleri ve bileşenleri kendin yazma;
`internal/ui` paketini kullan. Bütün yerleşik eklentiler (Site, Yönetici Paneli, DeltaX,
konu_ac, Pembiş) bu paketi kullanır. Paket, AND'ın etkin temasına göre boyanır.

```go
import "github.com/lucian95511/and/internal/ui"

func main() {
    client, err := pluginapi.NewClientFromEnv()
    if err != nil {
        os.Exit(1)
    }
    // AND'ın etkin temasını uygula. Eski bir AND'da uç nokta yoksa varsayılan tema kalır.
    if tema, err := client.GetTheme(); err == nil {
        ui.Apply(ui.Theme(tema))
    }
    // ...
}

func (m model) View() string {
    w := ui.ContentWidth(m.width) // tam ekran çerçevenin içindeki kullanılabilir genişlik
    var b strings.Builder
    b.WriteString(ui.Header(w, []string{"Örnek", "Liste"}, ui.S.Muted.Render("3 öğe")) + "\n\n")
    for i, it := range m.items {
        b.WriteString(ui.Row(w, i == m.sel, ui.T(it.ad), ui.M("  "+it.aciklama)) + "\n")
    }

    // Alt bilgi her zaman ekranın en altında durur.
    alt := ui.Divider(w) + "\n"
    if m.bildirim != "" {
        alt += ui.Notice(ui.KindSuccess, m.bildirim) + "\n"
    }
    alt += ui.KeyHints(w, ui.H("↑↓", "seç"), ui.H("enter", "aç"), ui.H("esc", "geri"))
    return ui.Page(m.width, m.height, b.String(), alt)
}
```

| Bileşen | Ne için |
|---|---|
| `ui.Page`, `ui.ContentWidth`, `ui.ContentHeight` | Terminalin tamamını kaplayan çerçeve: gövde üstte, alt bilgi en altta; üst kenarda `AND` rozeti ve tema adı. `ui.Frame` alt bilgisiz kısayoludur |
| `ui.Measure` | Uzun düz yazının sarılacağı okuma genişliği (en fazla 100 sütun) |
| `ui.Spread`, `ui.Block`, `ui.VLine` | Sola ve sağa yaslı satır, sabit boyutlu blok, dikey ayraç: yan yana paneller kurmak için |
| `ui.Header` | "Başlık › Alt › Konu" yolu, sağda kısa durum metni, altında ayraç |
| `ui.Row`, `ui.T`, `ui.M`, `ui.C` | Liste satırı: seçiliyse tam genişlik zemin ve `▌` vurgu çubuğu; taşan metin `…` ile kırpılır |
| `ui.KeyHints`, `ui.H` | Alttaki "tuş açıklama" satırı; sığmayan çift bölünmeden alt satıra geçer |
| `ui.Notice` | Tek satır bildirim: `KindSuccess` ✓, `KindError` ✗, `KindWarning` !, `KindPending` … |
| `ui.Tabs`, `ui.KV`, `ui.Section`, `ui.Bar` | Sekme şeridi, hizalı "etiket değer" satırı, bölüm başlığı, doluluk çubuğu |
| `ui.StyleInput`, `ui.StyleTextarea` | bubbles giriş kutularını tema renkleriyle stillendirir |
| `ui.S.*`, `ui.P.*` | Hazır stiller (`Title`, `Muted`, `Selected`, `Success`…) ve ham palet renkleri |

Kendi `lipgloss.Style` değişkenlerin varsa bunları `ui.S`'den türeten bir fonksiyonda kur ve
`ui.Apply`'dan sonra yeniden çağır (bu dizindeki `main.go`'da `stilleriKur`). Paket
düzeyinde bir kez kurulan stil, temanın değiştiğini görmez.

---

## Gizli Eklenti — Forumdan Açılan

`label: ""` olan eklentiler ana menüde görünmez. Bu tür eklentiler AND tarafından doğrudan başlatılır.

`konu_ac` eklentisi bu modeli kullanır:
- Kullanıcı forumda `n` tuşuna bastığında AND, `AND_CATEGORY=<seçili_kategori>` ile `and-plugin-konu-ac`'ı başlatır.
- Eklenti bu değişkeni okur ve kategori seçim ekranını atlar.

```go
preCategory := pluginapi.Category()  // AND_CATEGORY ortam değişkeni
if preCategory != "" {
    // Kategori seçim ekranını atla, doğrudan forma geç
}
```

> Gizli eklentiler de `plugin.json` gerektirmez — sadece binary yeterlidir.
> Ancak kullanıcı `plugins_state.json`'da devre dışı bırakabilir.

---

## AND_DATA_DIR — Yerel Depolama

```go
dataDir := pluginapi.DataDir()  // AND_DATA_DIR ortam değişkeni

path := filepath.Join(dataDir, "meklentim_ayarlar.json")
os.WriteFile(path, data, 0o600)
```

Bu dizine yazdığın dosyalar `.gitignore`'a eklenmeli; kimlik veya anahtar içermemelidir.

---

## Keşif Kuralları

AND şu kurallara uyan binary'leri eklenti olarak tanır:

1. Dosya adı `and-plugin-` ile başlamalıdır
2. Windows'ta `.exe` uzantısı olabilir
3. Binary'nin yanında `and-plugin-<name>.json` sidecar varsa önce o okunur
4. Sidecar yoksa `--manifest` ile çalıştırılarak JSON meta veri alınır
5. JSON'daki `name` alanı boş olmamalıdır
6. Binary AND yürütülebilir dosyasıyla **aynı dizinde** bulunmalıdır

---

## Açma / Kapama

Kullanıcı menüde bir eklentinin üzerine gelip **space** tuşuna basarak açıp kapatabilir:

- Açık eklentiler: normal görüntülenir, enter ile başlatılır
- Kapalı eklentiler: `[kapalı]` badge'i ile gri gösterilir, enter çalışmaz
- Durum `<AND_DATA_DIR>/plugins_state.json` dosyasına kaydedilir

AND yeniden başlatıldığında durum korunur.

---

## Test Etme

```bash
# Sidecar JSON'u doğrula
cat and-plugin-meklentim.json

# --manifest fallback'ini doğrula (sidecar yokken)
./and-plugin-meklentim --manifest
# Beklenen: {"name":"meklentim","label":"Benim Eklentim",...}
```

API entegrasyon testi için `pluginapi.NewServer` ile test sunucusu başlatabilirsin:

```go
srv := pluginapi.NewServer(mockID, mockForum, nil)
addr, _ := srv.Start(ctx)
t.Setenv("AND_API_ADDR", addr)
client, _ := pluginapi.NewClientFromEnv()
// ... test et
```

---

## Referans

- [pluginapi paketi](../../internal/pluginapi/api.go) — API sunucusu, istemci ve tüm tip tanımları
- [pluginmgr paketi](../../internal/pluginmgr/manager.go) — keşif, sidecar JSON okuma, açma/kapama mantığı
- [Admin eklentisi](../admin/) — gerçek eklenti örneği (plugin.json + main.go)
- [Ozel chat eklentisi](../ozel_chat/) — DM long-poll örneği
