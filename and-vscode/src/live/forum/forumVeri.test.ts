import { test } from "node:test";
import assert from "node:assert/strict";
import { LiveApiError } from "../liveClient";
import { PostSummary } from "../liveTypes";
import { kategoriSayilari, konuListesiYukle, siralaKonular } from "./forumVeri";

const konu = (id: string, ek: Partial<PostSummary> = {}): PostSummary => ({
  id,
  category: "Genel",
  title: "Konu " + id,
  author_name: "ali",
  author_key: "a".repeat(64),
  ...ek,
});

test("siralaKonular: son etkinliğe, yoksa oluşturulmaya göre yeniden eskiye; zamansızlar sonda", () => {
  const sirali = siralaKonular([
    konu("eski", { created_at: "2026-11-20T10:00:00Z" }),
    konu("zamansiz"),
    konu("yanitli", { created_at: "2026-11-19T10:00:00Z", last_activity: "2026-11-25T09:00:00Z" }),
    konu("yeni", { created_at: "2026-11-24T10:00:00Z", last_activity: "2026-11-24T10:00:00Z" }),
  ]);
  assert.deepEqual(
    sirali.map((k) => k.id),
    ["yanitli", "yeni", "eski", "zamansiz"]
  );
});

test("siralaKonular girdiyi değiştirmez", () => {
  const girdi = [konu("a", { created_at: "2026-11-20T10:00:00Z" }), konu("b", { created_at: "2026-11-21T10:00:00Z" })];
  siralaKonular(girdi);
  assert.deepEqual(
    girdi.map((k) => k.id),
    ["a", "b"]
  );
});

test("kategoriSayilari: AND'ın kategori sırasında, yalnızca konusu olanlar; bilinmeyen kategori sonda", () => {
  const sayilar = kategoriSayilari([
    konu("1", { category: "Genel" }),
    konu("2", { category: "Python" }),
    konu("3", { category: "Genel" }),
    konu("4", { category: "Uzaylı" }),
  ]);
  assert.deepEqual(sayilar, [
    { kategori: "Python", sayi: 1 },
    { kategori: "Genel", sayi: 2 },
    { kategori: "Uzaylı", sayi: 1 },
  ]);
});

const istemci = (posts: () => Promise<PostSummary[]>, hepsi: () => Promise<PostSummary[]>) => ({
  listPosts: posts,
  getAllPosts: hepsi,
});

test("konuListesiYukle: yeni AND'da herkes konu listesini alır, sıralı", async () => {
  const r = await konuListesiYukle(
    istemci(
      async () => [konu("a", { created_at: "2026-11-20T10:00:00Z" }), konu("b", { created_at: "2026-11-22T10:00:00Z" })],
      async () => assert.fail("eski uç çağrılmamalı")
    ),
    false
  );
  assert.equal(r.liste, true);
  assert.deepEqual(
    r.konular.map((k) => k.id),
    ["b", "a"]
  );
});

test("konuListesiYukle: eski AND'da (404) moderatör tüm konular ucuna düşer, üye listesiz kalır", async () => {
  const eski = async (): Promise<PostSummary[]> => {
    throw new LiveApiError("HTTP 404", 404);
  };
  const mod = await konuListesiYukle(istemci(eski, async () => [konu("m")]), true);
  assert.deepEqual(mod, { konular: [konu("m")], liste: true });
  const uye = await konuListesiYukle(istemci(eski, async () => assert.fail("üye eski ucu çağırmamalı")), false);
  assert.deepEqual(uye, { konular: [], liste: false });
});

test("konuListesiYukle: 404 dışındaki hatalar yukarı iletilir", async () => {
  await assert.rejects(
    konuListesiYukle(
      istemci(
        async () => {
          throw new LiveApiError("sunucu", 500);
        },
        async () => []
      ),
      true
    ),
    /sunucu/
  );
});
