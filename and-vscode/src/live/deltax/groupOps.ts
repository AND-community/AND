import { Grup, Uye } from "../../types";
import type { PeerInfo } from "../liveTypes";

/**
 * Davet listesinde önerilecek eşler. Gruptaki üyeler çıkarılır. Bağlı
 * eşlerin çoğu genel DHT düğümüdür ve DeltaX mesajı alamaz; AND en az bir
 * eşi `and: true` ile işaretlediyse yalnızca AND düğümleri önerilir. Alanı
 * hiç göndermeyen eski bir AND'da (ya da bağlı AND düğümü yokken) tüm eşler
 * kalır — elle Peer ID girme seçeneği her durumda açıktır.
 */
export function inviteCandidates(peers: PeerInfo[], g: Grup): PeerInfo[] {
  const known = new Set(g.uyeler.map((u) => u.peer_id));
  const rest = peers.filter((p) => !known.has(p.peer_id));
  return peers.some((p) => p.and) ? rest.filter((p) => p.and) : rest;
}

// store.go'daki Grup üstünde tanımlı yöntemlerin (benKurucuMuyum/yoneticiMi/
// yetkiliMi/uyeMi/uyeEkleGuncelle/uyeCikar/yoneticiDurumunuDegistir) saf
// fonksiyon karşılıkları — hem syncService.ts (gelen protokol mesajlarını
// işlerken) hem groupActions.ts (komut gating'i) tarafından kullanılır.

export function isFounder(g: Grup, peerId: string): boolean {
  return g.kurucu_peer === peerId;
}

export function isAdmin(g: Grup, peerId: string): boolean {
  return (g.yoneticiler ?? []).some((y) => y.peer_id === peerId);
}

/** yetkiliMi — kurucu ya da yönetici mi (üye davet etme/grubu kapatma yetkisi). */
export function isAuthorized(g: Grup, peerId: string): boolean {
  return isFounder(g, peerId) || isAdmin(g, peerId);
}

export function isMember(g: Grup, peerId: string): boolean {
  return g.uyeler.some((u) => u.peer_id === peerId);
}

/** uyeEkleGuncelle — üyeyi ekler ya da peer ID eşleşiyorsa bilgilerini günceller (grubu yerinde değiştirir). */
export function upsertMember(g: Grup, u: Uye): void {
  const idx = g.uyeler.findIndex((existing) => existing.peer_id === u.peer_id);
  if (idx === -1) {
    g.uyeler.push(u);
  } else {
    g.uyeler[idx] = u;
  }
}

/** uyeCikar */
export function removeMember(g: Grup, peerId: string): void {
  g.uyeler = g.uyeler.filter((u) => u.peer_id !== peerId);
}

/** yoneticiDurumunuDegistir — kurucuyu bu şekilde yönetici yapmaz/kaldırmaz (ayrıca isFounder ile kontrol edilmeli). */
export function setAdmin(g: Grup, u: Uye, adminNow: boolean): void {
  const list = g.yoneticiler ?? (g.yoneticiler = []);
  const idx = list.findIndex((y) => y.peer_id === u.peer_id);
  if (idx !== -1) {
    if (!adminNow) {
      list.splice(idx, 1);
    }
    return;
  }
  if (adminNow) {
    list.push(u);
  }
}
