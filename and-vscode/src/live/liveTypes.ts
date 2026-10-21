// internal/pluginapi/api.go içindeki JSON tiplerinin birebir karşılığı.
// Bu dosya yalnızca AND'ın canlı yerel API'sine (pipe/socket üzerinden,
// bkz. liveClient.ts) bağlıyken kullanılır — DeltaX'in kendi yerel dosya
// formatından bağımsızdır.

export interface IdentityInfo {
  name: string;
  pub_key: string;
  peer_id: string;
}

export interface RoleInfo {
  is_founder: boolean;
  is_moderator: boolean;
}

export interface PeerInfo {
  peer_id: string;
  addrs: string[];
  /** Eş AND protokolünü konuşuyor (identify ile). AND bu alanı yalnızca
   *  true iken gönderir; eski bir AND hiç göndermez. */
  and?: boolean;
}

export interface PostSummary {
  id: string;
  category: string;
  title: string;
  author_name: string;
  author_key: string;
}

export interface ReplyInfo {
  id: string;
  author_name: string;
  author_key: string;
  body: string;
  created_at: string;
}

export interface PostDetail {
  id: string;
  category: string;
  author_name: string;
  author_key: string;
  title: string;
  body: string;
  created_at: string;
  replies: ReplyInfo[];
}

export interface CreatePostReq {
  category: string;
  title: string;
  body: string;
}

export interface CreateReplyReq {
  post_id: string;
  body: string;
}

export interface SendDMReq {
  peer_id: string;
  message: string;
  /** Alıcıya şu an ulaşılamıyorsa AND mesajı giden kutusunda bekletir ve
   *  alıcı bağlandığında iletir (7 güne kadar). Eski bir AND yok sayar. */
  queue_if_offline?: boolean;
}

export interface DMMsg {
  /** Görünen ad: gönderenin KENDİ beyan ettiği ad ve Peer ID'sinin kısaltması
   *  ("ali (12D3KooWAbCd…)") ya da yalnızca Peer ID. Kimlik doğrulaması için
   *  KULLANILMAMALI — bkz. verified_peer_id. */
  from: string;
  /** Gönderenin bağlantıdan doğrulanmış tam Peer ID'si. Çok eski bir AND göndermez. */
  verified_peer_id?: string;
  text: string;
  received_at: string;
}

/** GET /api/v1/dm/history — bu düğümün gördüğü bir özel mesaj (gelen ya da giden), geliş sırasıyla. */
export interface DMRecord {
  /** Konuşulan eş: gelende bağlantıdan doğrulanmış gönderen, gidende alıcı. */
  peer_id: string;
  /** Gelende gönderenin beyan ettiği ad (doğrulanmamış). */
  name?: string;
  text: string;
  /** Bu düğümün saatiyle alma/gönderme anı. */
  at: string;
  outgoing?: boolean;
  /** Kaydın kimliği; eski bir AND göndermez. */
  id?: string;
  /** Giden kutusunda alıcının bağlanmasını bekliyor. */
  pending?: boolean;
  /** Giden kutusunda süresi doldu, gönderilemedi. */
  failed?: boolean;
}

export interface SendChatReq {
  message: string;
}

export interface ChatMsg {
  /** Yalnızca /api/v1/chat/all'dan gelen geçmiş mesajlarda dolu; canlı poll akışında yoktur. */
  id?: string;
  from: string;
  text: string;
  sent_at: string;
}

/** api.go'daki SitePageInfo — her zaman ÇAĞIRANIN kendi sayfasıdır (bkz. SiteBackend), bu yüzden yazar alanı yok. */
export interface SitePageInfo {
  slug: string;
  title: string;
  body: string;
  updated_at: string;
}

/** POST /api/v1/site/publish — slug yeniyse oluşturur, varsa sayfayı TAMAMEN değiştirir (kısmi güncelleme yok). */
export interface PublishSitePageReq {
  slug: string;
  title: string;
  body: string;
}

export interface SendFileReq {
  peer_id: string;
  local_path: string;
}

export interface FileMsg {
  from: string;
  filename: string;
  size: number;
  save_path: string;
  received_at: string;
}

export interface FileConsentReq {
  transfer_id: string;
  sender_id: string;
  filename: string;
  size: number;
}

export interface ConsentResp {
  transfer_id: string;
  accept: boolean;
}

/** internal/pluginapi/api.go'daki ThemeReq ile birebir aynı. Renk değerleri
 * terminal palet numarası ("63") ya da hex kod ("#ff6600") olabilir — bkz.
 * themeColor.ts'deki ansiOrHexToCss(). */
export interface ThemeReq {
  theme_name?: string;
  accent?: string;
  sel_bg?: string;
  sel_fg?: string;
  muted?: string;
  name?: string;
  badge?: string;
}

export interface ErrResp {
  error: string;
}

/** AND'ın kapalı forum kategori kümesi, aynı sırayla (protokol/kategori.go'daki
 * Kategoriler). AND bu listede olmayan kategoriyle gelen konuyu reddeder;
 * liveTypes.test.ts listeyi Go kaynağıyla karşılaştırır. */
export const FORUM_CATEGORIES = [
  "Python",
  "C / C++",
  "Rust",
  "Go",
  "JavaScript",
  "Java / Kotlin",
  "C# / .NET",
  "PHP",
  "Swift / Objective-C",
  "Yazılım",
  "Web",
  "Mobil",
  "Yapay Zeka",
  "Veritabanı",
  "DevOps",
  "Bulut Bilişim",
  "Linux",
  "Bilişim",
  "Ağ ve Sistem Yönetimi",
  "Siber Güvenlik",
  "Donanım",
  "Gömülü Sistemler / IoT",
  "Robotik",
  "Blockchain / Web3",
  "Algoritma ve Veri Yapıları",
  "Test ve Kalite",
  "UI / UX Tasarım",
  "Oyun Geliştirme",
  "Açık Kaynak",
  "Kariyer",
  "Freelance",
  "Eğitim Kaynakları",
  "Etkinlikler ve Duyurular",
  "Genel",
];
