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
}

export interface DMMsg {
  from: string;
  text: string;
  received_at: string;
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

/** forum.go'daki Categories listesiyle birebir aynı — yeni konu oluştururken önerilir. */
export const FORUM_CATEGORIES = [
  "Python",
  "C / C++",
  "Rust",
  "Go",
  "JavaScript",
  "Java / Kotlin",
  "Yazılım",
  "Web",
  "Mobil",
  "Yapay Zeka",
  "Veritabanı",
  "DevOps",
  "Linux",
  "Bilişim",
  "Siber Güvenlik",
  "Donanım",
  "Oyun Geliştirme",
  "Açık Kaynak",
  "Kariyer",
  "Genel",
];
