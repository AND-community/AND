// AND/Eklentiler/DeltaX (Go) tarafındaki store.go / vcs.go içindeki JSON
// yapılarının bire bir karşılığı. Alan adları Go'nun json tag'leriyle
// aynı olmalı — aksi halde bu uzantı, DeltaX TUI'sinin yazdığı dosyaları
// okuyamaz/DeltaX'in okuyabileceği dosyalar yazamaz.

export interface Uye {
  ad: string;
  peer_id: string;
  pub_key?: string;
}

export interface Grup {
  id: string;
  ad: string;
  sir: string;
  kurucu_ad: string;
  kurucu_peer: string;
  uyeler: Uye[];
  yoneticiler?: Uye[];
  olusturma_ts: string;
}

export interface Proje {
  id: string;
  grup_id: string;
  ad: string;
  olusturma_ts: string;
  dallar: Record<string, string>;
  aktif_dal: string;
  uzaklar?: Record<string, string>;
}

export interface AgacGirdi {
  yol: string;
  hash: string;
  boyut: number;
}

export interface Agac {
  girdiler: AgacGirdi[] | null;
}

export interface Commit {
  agac: string;
  ebeveynler?: string[];
  yazar: string;
  mesaj: string;
  ts: string;
}

export interface DurumSonucu {
  eklenen: string[];
  degisen: string[];
  silinen: string[];
}

export function durumTemizMi(d: DurumSonucu): boolean {
  return d.eklenen.length === 0 && d.degisen.length === 0 && d.silinen.length === 0;
}

export interface LogGirdisi {
  hash: string;
  commit: Commit;
}
