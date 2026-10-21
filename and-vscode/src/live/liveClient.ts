import * as http from "http";
import {
  ChatMsg,
  ConsentResp,
  CreatePostReq,
  CreateReplyReq,
  DMMsg,
  DMRecord,
  ErrResp,
  FileConsentReq,
  FileMsg,
  IdentityInfo,
  PeerInfo,
  PostDetail,
  PostSummary,
  PublishSitePageReq,
  RoleInfo,
  SendDMReq,
  SendFileReq,
  SitePageInfo,
  ThemeReq,
} from "./liveTypes";

// AND'ın yerel eklenti API'sine (internal/pluginapi/api.go) named pipe /
// Unix domain socket üzerinden konuşan ince bir HTTP istemcisi. Bearer
// token YOKTUR — erişim zaten OS tarafından (yalnızca bu kullanıcı)
// kısıtlanmıştır (bkz. liveAddr.ts, internal/pluginapi/externalpipe_*.go).

export class LiveApiError extends Error {
  constructor(message: string, readonly status?: number) {
    super(message);
  }
}

const DEFAULT_TIMEOUT_MS = 10_000;
const POLL_TIMEOUT_MS = 40_000; // sunucu long-poll'u en fazla 5sn tutar; bolca pay bırakılır

function request<T>(socketPath: string, method: string, urlPath: string, body?: unknown, timeoutMs = DEFAULT_TIMEOUT_MS): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const payload = body !== undefined ? Buffer.from(JSON.stringify(body), "utf8") : undefined;
    const req = http.request(
      {
        socketPath,
        path: urlPath,
        method,
        headers: payload
          ? { "Content-Type": "application/json", "Content-Length": payload.length }
          : undefined,
        timeout: timeoutMs,
      },
      (res) => {
        const chunks: Buffer[] = [];
        res.on("data", (chunk: Buffer) => chunks.push(chunk));
        res.on("end", () => {
          const raw = Buffer.concat(chunks).toString("utf8");
          const status = res.statusCode ?? 0;
          if (status === 204 || raw === "") {
            if (status >= 200 && status < 300) {
              resolve(undefined as T);
            } else {
              reject(new LiveApiError(`HTTP ${status}`, status));
            }
            return;
          }
          let parsed: unknown;
          try {
            parsed = JSON.parse(raw);
          } catch {
            reject(new LiveApiError(`geçersiz yanıt: ${raw.slice(0, 200)}`, status));
            return;
          }
          if (status >= 200 && status < 300) {
            resolve(parsed as T);
          } else {
            const msg = (parsed as ErrResp | undefined)?.error ?? `HTTP ${status}`;
            reject(new LiveApiError(msg, status));
          }
        });
      }
    );
    req.on("timeout", () => req.destroy(new Error("istek zaman aşımına uğradı")));
    req.on("error", reject);
    if (payload) {
      req.write(payload);
    }
    req.end();
  });
}

export class LiveClient {
  constructor(private readonly socketPath: string) {}

  private get<T>(path: string, timeoutMs?: number): Promise<T> {
    return request<T>(this.socketPath, "GET", path, undefined, timeoutMs);
  }

  private post<T>(path: string, body?: unknown, timeoutMs?: number): Promise<T> {
    return request<T>(this.socketPath, "POST", path, body, timeoutMs);
  }

  getIdentity(): Promise<IdentityInfo> {
    return this.get("/api/v1/identity");
  }

  getRole(): Promise<RoleInfo> {
    return this.get("/api/v1/role");
  }

  getPeers(): Promise<PeerInfo[]> {
    return this.get("/api/v1/peers");
  }

  /** Yalnızca kurucu/moderatör. */
  getAllPosts(): Promise<PostSummary[]> {
    return this.get("/api/v1/forum/all-posts");
  }

  getPost(id: string): Promise<PostDetail> {
    return this.get(`/api/v1/forum/post?id=${encodeURIComponent(id)}`);
  }

  createPost(req: CreatePostReq): Promise<void> {
    return this.post("/api/v1/forum/post", req);
  }

  createReply(req: CreateReplyReq): Promise<void> {
    return this.post("/api/v1/forum/reply", req);
  }

  deletePost(postId: string): Promise<void> {
    return this.post("/api/v1/forum/delete", { post_id: postId });
  }

  sendDM(req: SendDMReq): Promise<void> {
    return this.post("/api/v1/dm/send", req);
  }

  /**
   * Sunucu tarafında en fazla 5sn tutan long-poll. Yalnızca istek açıkken
   * gelenleri görür; mesaj kaçırmamak için getDMHistory kullanılır, bu yalnızca
   * geçmişi olmayan eski bir AND içindir.
   */
  pollDM(): Promise<DMMsg[]> {
    return this.get("/api/v1/dm/poll", POLL_TIMEOUT_MS);
  }

  /**
   * AND'ın DM geçmişi (gelen ve giden, geliş sırasıyla). from verilirse
   * yalnızca o kimlikteki kayıt ve sonrakiler gelir; kayıt artık geçmişte
   * yoksa ya da AND bu süzmeyi bilmiyorsa tamamı gelir. Geçmişi olmayan eski
   * bir AND'da 404 döner.
   */
  getDMHistory(from?: string): Promise<DMRecord[]> {
    return this.get(from ? `/api/v1/dm/history?from=${encodeURIComponent(from)}` : "/api/v1/dm/history");
  }

  /** Konuşma başına okundu zamanları (Peer ID → zaman). Okundu bilgisi tutmayan eski bir AND'da 404 döner. */
  getDMReadMarks(): Promise<Record<string, string>> {
    return this.get("/api/v1/dm/read");
  }

  /** peerId ile konuşmanın at'a kadar okunduğunu AND'a bildirir (TUI'deki rozet ve Özel Chat aynı bilgiyi görür). */
  markDMRead(peerId: string, at: string): Promise<void> {
    return this.post("/api/v1/dm/read", { peer_id: peerId, at });
  }

  sendChat(message: string): Promise<void> {
    return this.post("/api/v1/chat/send", { message });
  }

  pollChat(): Promise<ChatMsg[]> {
    return this.get("/api/v1/chat/poll", POLL_TIMEOUT_MS);
  }

  /** Yerelde bilinen TÜM sohbet geçmişi (kalıcı) — pollChat'in aksine yalnızca yeni gelenler değil. */
  getAllChat(): Promise<ChatMsg[]> {
    return this.get("/api/v1/chat/all");
  }

  /** Çağıranın KENDİ kişisel site sayfaları. */
  getSitePages(): Promise<SitePageInfo[]> {
    return this.get("/api/v1/site/pages");
  }

  publishSitePage(req: PublishSitePageReq): Promise<void> {
    return this.post("/api/v1/site/publish", req);
  }

  deleteSitePage(slug: string): Promise<void> {
    return this.post("/api/v1/site/delete", { slug });
  }

  sendFile(req: SendFileReq): Promise<void> {
    return this.post("/api/v1/file/send", req);
  }

  /** Sunucu tarafında en fazla 5sn tutan long-poll; alınan dosyaları bildirir. */
  pollFile(): Promise<FileMsg[]> {
    return this.get("/api/v1/file/poll", POLL_TIMEOUT_MS);
  }

  /** Sunucu tarafında en fazla 5sn tutan long-poll; gelen dosya onay isteklerini bildirir. */
  pollConsent(): Promise<FileConsentReq[]> {
    return this.get("/api/v1/file/consent-poll", POLL_TIMEOUT_MS);
  }

  respondConsent(req: ConsentResp): Promise<void> {
    return this.post("/api/v1/file/consent", req);
  }

  /** AND'ın o an aktif temasını okur (ör. pembis eklentisiyle değiştirilmiş olabilir). */
  getTheme(): Promise<ThemeReq> {
    return this.get("/api/v1/theme");
  }
}
