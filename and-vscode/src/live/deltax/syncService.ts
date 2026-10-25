import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";
import { readProject, saveProject } from "../../groupProjectStore";
import { GelenDM } from "../dmCore";
import { LiveConnection } from "../liveConnection";
import { FileConsentReq, FileMsg } from "../liveTypes";
import { BekleyenDavet, bekleyenDavetler, davetGonderildi } from "./davetler";
import { fetchSelfUye } from "./identity";
import { applyIncomingPackFile, packSenderTrusted } from "./packSync";
import { ZarfOrtami, davetEden, davetiYanitla, zarfIsle } from "./zarfIsleyici";
import { t } from "../../dil/dil";

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * model.go'daki zarfIsle + dosya/onay işleyicilerinin VS Code katmanı.
 * extension.ts, AND'ın DM geçmişinden gelen her mesajı ÖNCE buraya iletir;
 * handleDM bir DeltaX zarfını tükettiyse `true` döner ve mesaj genel DM
 * bildirimine ya da Canlı Panel'e hiç düşmez. Zarfların işlenmesi
 * zarfIsleyici.ts'tedir (vscode'dan bağımsız, testli); burada yalnızca
 * bildirimler ve kullanıcıya sorulan sorular vardır.
 */
export class DeltaXSyncService implements vscode.Disposable {
  private readonly groupsChangedEmitter = new vscode.EventEmitter<void>();
  readonly onGroupsChanged = this.groupsChangedEmitter.event;
  private readonly ortam: ZarfOrtami;

  /** durumDosyasi(ad): bu AND veri dizinine özgü, VS Code'un depolama alanındaki durum dosyası. */
  constructor(
    private readonly getRoot: () => string,
    private readonly conn: LiveConnection,
    durumDosyasi: (ad: string) => string
  ) {
    this.ortam = {
      root: getRoot,
      defterDosyasi: () => durumDosyasi("deltax-gonderilen-davetler"),
      bekleyenDosyasi: () => durumDosyasi("deltax-bekleyen-davetler"),
      istemci: () => conn.getClient(),
      ben: async () => {
        const c = conn.getClient();
        return c ? fetchSelfUye(c) : undefined;
      },
      bildir: (mesaj, hata) => this.notify(mesaj, hata),
      davetGeldi: (d) => this.davetiSor(d),
      gruplarDegisti: () => this.groupsChangedEmitter.fire(),
      simdi: () => new Date(),
    };
  }

  dispose(): void {
    this.groupsChangedEmitter.dispose();
  }

  /**
   * Davet gönderilmeden HEMEN ÖNCE çağrılır: kabul bu kayda göre tanınır.
   * İyimser kayıttır; davet gidemezse kayıt süresi dolana kadar zararsızca
   * defterde kalır.
   */
  davetGonderiliyor(grupId: string, peerId: string): void {
    davetGonderildi(this.ortam.defterDosyasi(), grupId, peerId, new Date());
  }

  bekleyenDavetler(): BekleyenDavet[] {
    return bekleyenDavetler(this.ortam.bekleyenDosyasi(), new Date());
  }

  /** Bekleyen bir daveti kabul eder ya da reddeder; sonucu bildirir. */
  async davetiYanitla(d: BekleyenDavet, kabul: boolean): Promise<void> {
    try {
      const g = await davetiYanitla(this.ortam, d, kabul);
      this.notify(g ? t("\"%s\" grubuna katıldın.", g.ad) : t("Davet reddedildi."));
    } catch (err) {
      this.notify(t("Davet yanıtlanamadı: %s. Davet bekleyenlerde duruyor.", describeError(err)), true);
    }
  }

  private notify(message: string, isError = false): void {
    if (isError) {
      void vscode.window.showErrorMessage(`AND — ${message}`);
    } else {
      void vscode.window.showInformationMessage(`AND — ${message}`);
    }
  }

  /** Bildirim kapatılırsa davet kaybolmaz: "DeltaX: Bekleyen Davetler" ve gruplar görünümünden yanıtlanabilir. */
  private davetiSor(d: BekleyenDavet): void {
    const kabul = t("Kabul Et");
    const ret = t("Reddet");
    void vscode.window
      .showInformationMessage(t("Yeni grup daveti: %s (davet eden: %s)", d.payload.grup_ad, davetEden(d)), kabul, ret)
      .then((choice) => {
        if (choice === kabul || choice === ret) {
          void this.davetiYanitla(d, choice === kabul);
        }
      });
  }

  /** true dönerse mesaj bir DeltaX zarfıydı (genel DM bildirimine/Canlı Panel'e düşmemeli). */
  handleDM(dm: GelenDM): Promise<boolean> {
    return zarfIsle(this.ortam, dm);
  }

  /** projeag- önekli dosyalar dışındaki her şeyi olduğu gibi bırakır (Canlı Panel'in Dosyalar sekmesi kendi başına yönetir). */
  async handleFile(msg: FileMsg): Promise<void> {
    if (!path.basename(msg.filename).startsWith("projeag-")) {
      return;
    }
    const root = this.getRoot();
    try {
      // FileMsg.from, AND'ın bağlantıdan doğruladığı gönderen Peer ID'sidir.
      const applied = applyIncomingPackFile(root, msg.save_path, msg.from);
      try {
        const p = readProject(root, applied.projeId);
        if (!p.uzaklar) {
          p.uzaklar = {};
        }
        if (applied.manifest.head) {
          p.uzaklar[`${msg.from}/${applied.manifest.dal}`] = applied.manifest.head;
        }
        saveProject(root, p);
      } catch {
        // proje okunamadı — es geç, nesneler yine de yazıldı
      }
      this.notify(t("%d yeni nesne alındı (%s) — Uzak Dallar'dan birleştirebilirsin.", applied.objectCount, applied.manifest.dal));
      this.groupsChangedEmitter.fire();
    } catch (err) {
      this.notify(t("Paket işlenemedi: %s", describeError(err)), true);
    } finally {
      fs.rmSync(msg.save_path, { force: true });
    }
  }

  /** true dönerse istek tüketildi (otomatik kabul edildi — genel onay bildirimi atlanmalı). */
  async handleConsent(req: FileConsentReq): Promise<boolean> {
    if (!req.filename.startsWith("projeag-")) {
      return false;
    }
    if (!packSenderTrusted(this.getRoot(), req.filename, req.sender_id)) {
      return false; // bilinmeyen gönderen — genel onay akışına düşsün
    }
    const client = this.conn.getClient();
    if (!client) {
      return false;
    }
    try {
      await client.respondConsent({ transfer_id: req.transfer_id, accept: true });
    } catch {
      // yok say
    }
    return true;
  }
}
