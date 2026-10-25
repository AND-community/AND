import * as path from "path";
import * as vscode from "vscode";
import { commonAncestor, createCommit, headTreeOf, readCommit, shortHash } from "./commitStore";
import { markMergeHead, saveProject } from "./groupProjectStore";
import { applyThreeWayMerge } from "./mergeEngine";
import { DeltaXRepository } from "./scmController";
import { authorName } from "./settings";
import { k, t } from "./dil/dil";

async function confirmDestructiveCheckout(): Promise<boolean> {
  const devam = t("Devam Et");
  const secim = await vscode.window.showWarningMessage(
    t(
      "Dal değiştirme, çalışma klasöründeki TÜM içeriği siler ve hedef dalın kaydedilmiş sürümüyle değiştirir (.projeagignore ile hariç tutulan ya da hiç izlenmeyen dosyalar dahil). Bu, DeltaX TUI'sinin kendi davranışıyla birebir aynıdır. Devam edilsin mi?"
    ),
    { modal: true },
    devam
  );
  return secim === devam;
}

/**
 * Yıkıcı bir eylemden (checkoutTree/applyThreeWayMerge) HEMEN ÖNCE
 * çağrılmalıdır — bir onay penceresinden sonra da olsa, hiç onay penceresi
 * olmasa da. Bu çağrı ile eylem arasında (özellikle bir onay penceresi
 * varsa — insan hızında, dosya izleyicisinin 400ms debounce'ından çok daha
 * uzun sürebilen bir bekleme) kullanıcı bir dosya kaydetmiş olabilir; bu
 * yüzden durumu senkron olarak YENİDEN tazeler ve hâlâ temiz olduğunu
 * doğrular. Bu kontrolü her çağıran taraf ayrı ayrı elle tekrarlamak yerine
 * tek bir yerde tutmak, yeni bir yıkıcı komut/yol eklendiğinde bu adımın
 * unutulmasını zorlaştırır.
 */
async function recheckCleanBeforeDestructiveAction(repo: DeltaXRepository, islemAdi: string): Promise<boolean> {
  repo.refresh();
  if (!repo.isClean()) {
    void vscode.window.showErrorMessage(
      t(
        "Az önce yeni değişiklikler tespit edildi — %s güvenlik için iptal edildi. Değişiklikleri kaydedip tekrar deneyin.",
        t(islemAdi)
      )
    );
    return false;
  }
  return true;
}

export async function switchBranch(repo: DeltaXRepository): Promise<void> {
  const dallar = Object.keys(repo.proje.dallar).sort();
  if (dallar.length === 0) {
    void vscode.window.showInformationMessage(t("Bu projede hiç dal yok."));
    return;
  }
  const items = dallar.map((d) => ({
    label: d === repo.proje.aktif_dal ? `$(check) ${d}` : d,
    description: d === repo.proje.aktif_dal ? t("geçerli dal") : undefined,
    dal: d,
  }));
  const secim = await vscode.window.showQuickPick(items, { placeHolder: t("Geçiş yapılacak dalı seçin") });
  if (!secim) {
    return;
  }
  const hedef = secim.dal;
  if (hedef === repo.proje.aktif_dal) {
    return;
  }

  // Ön kontrol: kullanıcıyı gereksiz yere yıkıcı onay penceresiyle
  // uğraştırmamak için mevcut (henüz zorla tazelenmemiş) durumu kontrol et.
  if (!repo.isClean()) {
    void vscode.window.showErrorMessage(
      t("Çalışma klasöründe kaydedilmemiş değişiklikler var. Önce Kaydet (Commit) yapın, sonra dal değiştirin.")
    );
    return;
  }
  if (!(await confirmDestructiveCheckout())) {
    return;
  }
  if (!(await recheckCleanBeforeDestructiveAction(repo, k("dal değiştirme")))) {
    return;
  }

  const hedefHead = repo.proje.dallar[hedef] ?? "";
  const hedefTree = headTreeOf(repo.root, repo.proje.id, hedefHead);
  try {
    repo.checkout(hedefTree);
  } catch (err) {
    void vscode.window.showErrorMessage(t("Dal değiştirilemedi: %s", describeError(err)));
    return;
  }
  repo.proje.aktif_dal = hedef;
  saveProject(repo.root, repo.proje);
  repo.refresh();
  void vscode.window.showInformationMessage(t("Dal değiştirildi: %s", hedef));
}

export async function newBranch(repo: DeltaXRepository): Promise<void> {
  const ad = await vscode.window.showInputBox({
    prompt: t("Yeni dal adı"),
    validateInput: (v) => {
      const trimmed = v.trim();
      if (!trimmed) {
        return t("Dal adı boş olamaz");
      }
      if (repo.proje.dallar[trimmed]) {
        return t("Bu isimde bir dal zaten var");
      }
      return undefined;
    },
  });
  if (!ad) {
    return;
  }
  const trimmed = ad.trim();
  repo.proje.dallar[trimmed] = repo.currentHead();
  saveProject(repo.root, repo.proje);
  repo.refresh();
  void vscode.window.showInformationMessage(t("Dal oluşturuldu: %s (henüz geçiş yapılmadı)", trimmed));
}

/**
 * birlestirCmd'nin karşılığı. proje.uzaklar, TUI üzerinden daha önce
 * gönderilen/çekilen ve nesneleri zaten yerel depoda bulunan uzak dal
 * uçlarını (peerID/dal → commit hash) tutar — bu yüzden birleştirme tamamen
 * yerel olarak yapılabilir; ağ trafiği gerekmez.
 */
export async function mergeRemoteBranch(repo: DeltaXRepository): Promise<void> {
  const uzaklar = repo.proje.uzaklar ?? {};
  const anahtarlar = Object.keys(uzaklar);
  if (anahtarlar.length === 0) {
    void vscode.window.showInformationMessage(
      t("Bilinen bir uzak dal ucu yok. Önce AND TUI'sinden Gönder/Çek ile senkronize edin.")
    );
    return;
  }
  const items = anahtarlar.map((k) => ({ label: k, description: shortHash(uzaklar[k]) }));
  const secim = await vscode.window.showQuickPick(items, { placeHolder: t("Birleştirilecek uzak dal ucunu seçin") });
  if (!secim) {
    return;
  }
  const uzakHead = uzaklar[secim.label];

  // Ön kontrol: mevcut (henüz zorla tazelenmemiş) durumu kontrol et — hem
  // fast-forward hem üç yönlü birleştirme yolu için gereksiz bir tam tarama
  // yapmadan hızlı bir "zaten kirli" tespiti. Asıl (nihai) kontrol, her iki
  // yolun da kendi yıkıcı eyleminden hemen önce ayrıca yapılır (bkz.
  // aşağıdaki recheckCleanBeforeDestructiveAction çağrıları).
  if (!repo.isClean()) {
    void vscode.window.showErrorMessage(t("Önce mevcut değişiklikleri kaydedin."));
    return;
  }

  let uzakCommit;
  try {
    uzakCommit = readCommit(repo.root, repo.proje.id, uzakHead);
  } catch {
    void vscode.window.showErrorMessage(
      t("Uzak commit bulunamadı — önce AND TUI'sinden Gönder/Çek ile nesneleri alın.")
    );
    return;
  }

  const yerelHead = repo.currentHead();
  const temel = commonAncestor(repo.root, repo.proje.id, yerelHead, uzakHead);
  if (temel === uzakHead) {
    void vscode.window.showInformationMessage(t("Zaten güncel."));
    return;
  }

  if (temel === yerelHead) {
    if (!(await confirmDestructiveCheckout())) {
      return;
    }
    if (!(await recheckCleanBeforeDestructiveAction(repo, k("birleştirme")))) {
      return;
    }
    try {
      repo.checkout(uzakCommit.agac);
    } catch (err) {
      void vscode.window.showErrorMessage(t("Birleştirme (fast-forward) başarısız: %s", describeError(err)));
      return;
    }
    repo.proje.dallar[repo.proje.aktif_dal] = uzakHead;
    saveProject(repo.root, repo.proje);
    repo.refresh();
    void vscode.window.showInformationMessage(t("İleri sarma (fast-forward) tamamlandı: %s", shortHash(uzakHead)));
    return;
  }

  // Bu yolda kullanıcıya sorulan bir onay penceresi yok, ama yukarıdaki ön
  // kontrolden bu yana geçen sürede (QuickPick seçimi vb.) dosya
  // izleyicisinin debounce'ı henüz tetiklenmemiş olabilir — asıl yıkıcı
  // eylemden (applyThreeWayMerge) hemen önce nihai kontrolü burada yap.
  if (!(await recheckCleanBeforeDestructiveAction(repo, k("birleştirme")))) {
    return;
  }

  const temelTree = headTreeOf(repo.root, repo.proje.id, temel);
  const yerelCommit = readCommit(repo.root, repo.proje.id, yerelHead);
  let catismalar: string[];
  try {
    ({ catismalar } = applyThreeWayMerge(
      repo.root,
      repo.proje.id,
      repo.calismaDir,
      temelTree,
      yerelCommit.agac,
      uzakCommit.agac
    ));
  } catch (err) {
    void vscode.window.showErrorMessage(t("Birleştirme uygulanamadı: %s", describeError(err)));
    return;
  }

  if (catismalar.length > 0) {
    markMergeHead(repo.root, repo.proje.id, uzakHead);
    repo.refresh();
    const catismaAc = t("Çakışan Dosyaları Aç");
    void vscode.window
      .showWarningMessage(
        t(
          "Birleştirme çakışmaları var (%d dosya). Dosyaları düzenleyip <<<<<<< / ======= / >>>>>>> işaretlerini kaldırdıktan sonra Kaydet ile tamamlayın.",
          catismalar.length
        ),
        catismaAc
      )
      .then(async (secim2) => {
        if (secim2 === catismaAc) {
          for (const yol of catismalar) {
            await vscode.window.showTextDocument(vscode.Uri.file(path.join(repo.calismaDir, ...yol.split("/"))), {
              preview: false,
            });
          }
        }
      });
    return;
  }

  const { hash: yeniTree, atlanan } = repo.buildWorkdirTree();
  const yeniHash = createCommit(
    repo.root,
    repo.proje.id,
    yeniTree,
    [yerelHead, uzakHead],
    authorName(),
    t("Birleştirme: %s", shortHash(uzakHead))
  );
  repo.proje.dallar[repo.proje.aktif_dal] = yeniHash;
  saveProject(repo.root, repo.proje);
  repo.refresh();
  void vscode.window.showInformationMessage(
    atlanan.length > 0
      ? t("Birleştirme tamamlandı: %s (%d dosya boyut sınırı nedeniyle atlandı)", shortHash(yeniHash), atlanan.length)
      : t("Birleştirme tamamlandı: %s", shortHash(yeniHash))
  );
}

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
