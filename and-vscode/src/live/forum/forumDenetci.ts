// Canlı Panel'in Forum sekmesinin uzantı tarafı: webview'dan gelen forum
// iletilerini AND'ın eklenti API'sine çevirir, açık konuyu ve canlı
// değişimleri izler. "vscode" modülüne bağımlı değildir (birim testli);
// livePanel.ts onu panele bağlar.

import { t } from "../../dil/dil";
import { LiveApiError } from "../liveClient";
import { BanReq, CreatePostReq, CreateReplyReq, PostDetail, ProfileInfo } from "../liveTypes";
import { DegisimIstemcisi, ForumDegisimi, ForumIzleyici } from "./forumIzleyici";
import { ForumListeIstemcisi, KonuListesi, kategoriSayilari, konuListesiYukle } from "./forumVeri";

/** Forum sekmesinin kullandığı AND uçları (LiveClient bunu karşılar). */
export interface ForumIstemcisi extends ForumListeIstemcisi, DegisimIstemcisi {
  getPost(id: string): Promise<PostDetail>;
  createPost(req: CreatePostReq): Promise<void>;
  createReply(req: CreateReplyReq): Promise<void>;
  deletePost(postId: string): Promise<void>;
  deleteOwnPost(postId: string): Promise<void>;
  getProfile(key: string): Promise<ProfileInfo>;
  ban(req: BanReq): Promise<void>;
}

/** Webview'a ileti gönderme ve kullanıcıya onay sorma (livePanel.ts sağlar). */
export interface ForumPaneli {
  gonder(msg: unknown): void;
  /** Değiştirilemez bir işlemden önce sorar; dugme seçilirse true. */
  onayla(soru: string, dugme: string): Promise<boolean>;
}

/** Kullanıcının forumdaki konumu: moderatör mü, genel anahtarı ne. */
export interface ForumRolu {
  moderator: boolean;
  anahtar?: string;
}

type Ileti = { type: string; [k: string]: unknown };

/** Bu denetçinin işlediği webview iletileri. */
const FORUM_ILETILERI = new Set(["forumOpen", "createPost", "reply", "deletePost", "profile", "ban"]);

function hataMetni(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export class ForumDenetci {
  /** Webview'da açık olan konu: canlı değişimde yeniden gönderilir. */
  private acikKonu: string | undefined;
  private readonly izleyici: ForumIzleyici;

  constructor(
    private readonly istemci: () => ForumIstemcisi | undefined,
    private readonly panel: ForumPaneli,
    private readonly rol: () => ForumRolu
  ) {
    this.izleyici = new ForumIzleyici(
      {
        forumChanges: (since) => {
          const c = this.istemci();
          return c ? c.forumChanges(since) : Promise.reject(new Error(t("AND'a canlı bağlantı yok.")));
        },
      },
      (d) => void this.degisimIsle(d).catch(() => undefined)
    );
  }

  /** Canlı izlemeyi başlatır (zaten çalışıyorsa bir şey yapmaz). */
  izlemeyiBaslat(): void {
    this.izleyici.baslat();
  }

  izlemeyiDurdur(): void {
    this.izleyici.durdur();
  }

  /** Konu listesini ve kategori sayılarını webview'a gönderir. */
  async listeyiGonder(): Promise<KonuListesi | undefined> {
    const c = this.istemci();
    if (!c) {
      return undefined;
    }
    const l = await konuListesiYukle(c, this.rol().moderator);
    this.panel.gonder({ type: "forumPosts", konular: l.konular, kategoriler: kategoriSayilari(l.konular), liste: l.liste });
    return l;
  }

  /** Forumda değişiklik oldu: liste ve (değiştiyse) açık konu yenilenir. */
  async degisimIsle(d: ForumDegisimi): Promise<void> {
    await this.listeyiGonder();
    if (this.acikKonu && (d.hepsi || d.konular.includes(this.acikKonu))) {
      await this.konuyuGonder(this.acikKonu, false);
    }
  }

  /**
   * Webview'dan gelen bir forum iletisini işler; forum iletisi değilse false
   * döner. Hatalar webview'a "error" olarak gider.
   */
  async mesaj(m: Ileti): Promise<boolean> {
    if (!FORUM_ILETILERI.has(m.type)) {
      return false;
    }
    try {
      await this.isle(m);
    } catch (err) {
      this.panel.gonder({ type: "error", message: hataMetni(err) });
    }
    return true;
  }

  /**
   * Konuyu gönderir. ac doğruysa kullanıcı açtı (webview konuya geçer);
   * değilse canlı yenilemedir (webview yalnızca o konu açıksa yeniden çizer).
   */
  private async konuyuGonder(id: string, ac: boolean): Promise<void> {
    const c = this.gerekli();
    try {
      const data = await c.getPost(id);
      this.panel.gonder(ac ? { type: "postDetail", data, ac: true } : { type: "postDetail", data });
    } catch (err) {
      if (err instanceof LiveApiError && err.status === 404) {
        this.panel.gonder(ac ? { type: "postGone", id, ac: true } : { type: "postGone", id });
        return;
      }
      throw err;
    }
  }

  private gerekli(): ForumIstemcisi {
    const c = this.istemci();
    if (!c) {
      throw new Error(t("AND'a canlı bağlantı yok."));
    }
    return c;
  }

  private async isle(m: Ileti): Promise<void> {
    const c = this.gerekli();
    switch (m.type) {
      case "forumOpen": {
        this.acikKonu = String(m.id);
        await this.konuyuGonder(this.acikKonu, true);
        return;
      }
      case "createPost": {
        const title = String(m.title);
        await c.createPost({ category: String(m.category), title, body: String(m.body) });
        this.panel.gonder({ type: "forumCreated" });
        this.panel.gonder({ type: "info", message: t("Gönderi oluşturuldu ve ağa yayınlandı.") });
        // AND yeni konunun kimliğini dönmez: listede kendi, aynı başlıklı en yeni konu açılır.
        const l = await this.listeyiGonder();
        const ben = this.rol().anahtar?.toLowerCase();
        const yeni = ben ? l?.konular.find((k) => k.title === title && k.author_key.toLowerCase() === ben) : undefined;
        if (yeni) {
          this.acikKonu = yeni.id;
          await this.konuyuGonder(yeni.id, true);
        }
        return;
      }
      case "reply": {
        const postId = String(m.postId);
        await c.createReply({ post_id: postId, body: String(m.body) });
        this.panel.gonder({ type: "replySent", postId });
        this.acikKonu = postId;
        await this.konuyuGonder(postId, false);
        return;
      }
      case "deletePost": {
        const postId = String(m.postId);
        if (!(await this.panel.onayla(t("“%s” konusu herkes için silinsin mi?", String(m.title ?? postId)), t("Sil")))) {
          return;
        }
        if (m.own === true) {
          await c.deleteOwnPost(postId);
        } else {
          await c.deletePost(postId);
        }
        if (this.acikKonu === postId) {
          this.acikKonu = undefined;
        }
        this.panel.gonder({ type: "postDeleted", id: postId });
        this.panel.gonder({ type: "info", message: t("Konu silindi.") });
        await this.listeyiGonder();
        return;
      }
      case "profile": {
        this.panel.gonder({ type: "profile", data: await c.getProfile(String(m.key)) });
        return;
      }
      case "ban": {
        const reason = String(m.reason ?? "").trim();
        if (!reason) {
          this.panel.gonder({ type: "error", message: t("Sebep boş olamaz.") });
          return;
        }
        const key = String(m.key);
        const ad = String(m.name || key.slice(0, 12));
        if (!(await this.panel.onayla(t("%s yasaklansın mı?", ad), t("Yasakla")))) {
          return;
        }
        const req: BanReq = { author_key: key, reason };
        if (m.permanent === true) {
          req.permanent = true;
        } else {
          req.duration_seconds = Number(m.seconds);
        }
        await c.ban(req);
        this.panel.gonder({ type: "info", message: t("%s yasaklandı.", ad) });
        this.panel.gonder({ type: "profile", data: await c.getProfile(key) });
        return;
      }
    }
  }
}
