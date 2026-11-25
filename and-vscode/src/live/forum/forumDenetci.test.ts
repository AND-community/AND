import { test } from "node:test";
import assert from "node:assert/strict";
import { LiveApiError } from "../liveClient";
import { BanReq, CreatePostReq, ForumChanges, PostDetail, PostSummary, ProfileInfo } from "../liveTypes";
import { ForumDenetci, ForumIstemcisi } from "./forumDenetci";

const BEN = "b".repeat(64);
const ALI = "a".repeat(64);

const ozet = (id: string, ek: Partial<PostSummary> = {}): PostSummary => ({
  id,
  category: "Genel",
  title: "Konu " + id,
  author_name: "ali",
  author_key: ALI,
  created_at: "2026-11-20T10:00:00Z",
  ...ek,
});
const detay = (id: string): PostDetail => ({
  id,
  category: "Genel",
  title: "Konu " + id,
  author_name: "ali",
  author_key: ALI,
  body: "gövde",
  created_at: "2026-11-20T10:00:00Z",
  replies: [],
});

/** Sahte AND: çağrıları kaydeder; değişim ucu çağrılınca sonsuza dek bekler. */
function sahteAnd() {
  const cagrilar: string[] = [];
  const s = {
    cagrilar,
    konular: [ozet("k1")] as PostSummary[],
    konuYok: new Set<string>(),
    banlar: [] as BanReq[],
    profil: { key: ALI, name: "ali", post_count: 1, reply_count: 0 } as ProfileInfo,
    async listPosts(): Promise<PostSummary[]> {
      cagrilar.push("listPosts");
      return s.konular;
    },
    async getAllPosts(): Promise<PostSummary[]> {
      cagrilar.push("getAllPosts");
      return s.konular;
    },
    forumChanges(): Promise<ForumChanges> {
      return new Promise(() => {});
    },
    async getPost(id: string): Promise<PostDetail> {
      cagrilar.push("getPost " + id);
      if (s.konuYok.has(id)) {
        throw new LiveApiError("konu bulunamadı", 404);
      }
      return detay(id);
    },
    async createPost(req: CreatePostReq): Promise<void> {
      cagrilar.push("createPost " + req.title);
      s.konular = [ozet("yeni", { title: req.title, author_key: BEN, created_at: "2026-11-25T10:00:00Z" }), ...s.konular];
    },
    async createReply(req: { post_id: string; body: string }): Promise<void> {
      cagrilar.push("createReply " + req.post_id + " " + req.body);
    },
    async deletePost(id: string): Promise<void> {
      cagrilar.push("deletePost " + id);
    },
    async deleteOwnPost(id: string): Promise<void> {
      cagrilar.push("deleteOwnPost " + id);
    },
    async getProfile(key: string): Promise<ProfileInfo> {
      cagrilar.push("getProfile " + key);
      return s.profil;
    },
    async ban(req: BanReq): Promise<void> {
      cagrilar.push("ban");
      s.banlar.push(req);
    },
  };
  return s;
}

function kur(opts: { moderator?: boolean; onay?: boolean } = {}) {
  const and = sahteAnd();
  const giden: { type: string; [k: string]: unknown }[] = [];
  const sorular: string[] = [];
  const d = new ForumDenetci(
    () => and as ForumIstemcisi,
    {
      gonder: (m) => giden.push(m as { type: string }),
      onayla: async (soru) => {
        sorular.push(soru);
        return opts.onay ?? true;
      },
    },
    () => ({ moderator: !!opts.moderator, anahtar: BEN })
  );
  const tur = (t: string) => giden.filter((m) => m.type === t);
  return { and, giden, sorular, d, tur };
}

test("denetçi: liste sıralı gönderilir; konu açılınca ayrıntı gelir", async () => {
  const { d, tur, and } = kur();
  and.konular = [ozet("eski", { created_at: "2026-11-01T00:00:00Z" }), ozet("yeni", { created_at: "2026-11-24T00:00:00Z" })];
  await d.listeyiGonder();
  assert.deepEqual(
    (tur("forumPosts")[0].konular as PostSummary[]).map((k) => k.id),
    ["yeni", "eski"]
  );
  assert.equal(tur("forumPosts")[0].liste, true);
  assert.equal(await d.mesaj({ type: "forumOpen", id: "k1" }), true);
  assert.equal((tur("postDetail")[0].data as PostDetail).id, "k1");
});

test("denetçi: değişim açık konuyu içeriyorsa liste ve ayrıntı yenilenir, içermiyorsa yalnız liste", async () => {
  const { d, tur, and } = kur();
  await d.mesaj({ type: "forumOpen", id: "k1" });
  and.cagrilar.length = 0;
  await d.degisimIsle({ konular: ["baska"], hepsi: false });
  assert.deepEqual(and.cagrilar, ["listPosts"]);
  await d.degisimIsle({ konular: ["k1"], hepsi: false });
  assert.deepEqual(and.cagrilar, ["listPosts", "listPosts", "getPost k1"]);
  await d.degisimIsle({ konular: [], hepsi: true });
  assert.equal(and.cagrilar.filter((c) => c === "getPost k1").length, 2);
  assert.equal(tur("postDetail").length, 3);
});

test("denetçi: açık konu silinmişse postGone gider", async () => {
  const { d, tur, and } = kur();
  await d.mesaj({ type: "forumOpen", id: "k1" });
  and.konuYok.add("k1");
  await d.degisimIsle({ konular: ["k1"], hepsi: false });
  assert.deepEqual(tur("postGone"), [{ type: "postGone", id: "k1" }]);
});

test("denetçi: yeni konu açılınca forumCreated, liste ve kendi yeni konusu gelir", async () => {
  const { d, tur } = kur();
  await d.mesaj({ type: "createPost", category: "Genel", title: "Merhaba", body: "ilk" });
  assert.equal(tur("forumCreated").length, 1);
  assert.equal((tur("postDetail").at(-1)!.data as PostDetail).id, "yeni");
});

test("denetçi: yanıt gönderilince replySent ve güncel ayrıntı gelir", async () => {
  const { d, tur, and } = kur();
  await d.mesaj({ type: "reply", postId: "k1", body: "katılıyorum" });
  assert.ok(and.cagrilar.includes("createReply k1 katılıyorum"));
  assert.deepEqual(tur("replySent"), [{ type: "replySent", postId: "k1" }]);
  assert.equal((tur("postDetail").at(-1)!.data as PostDetail).id, "k1");
});

test("denetçi: silme önce sorulur; kendi konusu delete-own, moderatör delete ucuyla", async () => {
  const red = kur({ onay: false });
  await red.d.mesaj({ type: "deletePost", postId: "k1", own: true, title: "Konu k1" });
  assert.equal(red.sorular.length, 1);
  assert.ok(red.sorular[0].includes("Konu k1"));
  assert.ok(!red.and.cagrilar.some((c) => c.startsWith("delete")));

  const kendi = kur();
  await kendi.d.mesaj({ type: "deletePost", postId: "k1", own: true, title: "Konu k1" });
  assert.ok(kendi.and.cagrilar.includes("deleteOwnPost k1"));
  assert.deepEqual(kendi.tur("postDeleted"), [{ type: "postDeleted", id: "k1" }]);

  const mod = kur({ moderator: true });
  await mod.d.mesaj({ type: "deletePost", postId: "k1", own: false, title: "Konu k1" });
  assert.ok(mod.and.cagrilar.includes("deletePost k1"));
});

test("denetçi: profil istenir; yasaklama sorulur, süre ya da kalıcı gider, profil yenilenir", async () => {
  const { d, tur, and, sorular } = kur({ moderator: true });
  await d.mesaj({ type: "profile", key: ALI });
  assert.equal((tur("profile")[0].data as ProfileInfo).name, "ali");

  await d.mesaj({ type: "ban", key: ALI, name: "ali", seconds: 86400, reason: "  reklam  " });
  assert.equal(sorular.length, 1);
  assert.deepEqual(and.banlar[0], { author_key: ALI, duration_seconds: 86400, reason: "reklam" });
  assert.equal(tur("profile").length, 2);

  await d.mesaj({ type: "ban", key: ALI, name: "ali", permanent: true, reason: "spam" });
  assert.deepEqual(and.banlar[1], { author_key: ALI, permanent: true, reason: "spam" });
});

test("denetçi: boş sebepli yasak AND'a gitmez", async () => {
  const { d, and, tur } = kur({ moderator: true });
  await d.mesaj({ type: "ban", key: ALI, name: "ali", seconds: 3600, reason: "   " });
  assert.equal(and.banlar.length, 0);
  assert.equal(tur("error").length, 1);
});

test("denetçi: bağlantı yoksa forum iletisi hata verir, bilinmeyen ileti işlenmez", async () => {
  const giden: { type: string }[] = [];
  const d = new ForumDenetci(
    () => undefined,
    { gonder: (m) => giden.push(m as { type: string }), onayla: async () => true },
    () => ({ moderator: false })
  );
  assert.equal(await d.mesaj({ type: "forumOpen", id: "k1" }), true);
  assert.equal(giden[0].type, "error");
  assert.equal(await d.mesaj({ type: "sendChat" }), false);
});
