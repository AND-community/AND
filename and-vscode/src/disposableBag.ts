import * as vscode from "vscode";

// vscode.Disposable.from() argümansız çağrıldığında, dispose()'u hiçbir şey
// yapmayan (birleştirilecek alt disposable'ı olmayan) bir Disposable döner —
// VS Code SDK'sının kendi "no-op disposable" idiomu.
const NOOP_DISPOSABLE: vscode.Disposable = vscode.Disposable.from();

/**
 * disposable'ı context.subscriptions'a ekler ve onu unutmak (dispose ETMEDEN,
 * yalnızca referansını bırakmak) için bir "forget" callback'i döndürür.
 * `context.subscriptions` yalnızca uzantı deactivate olurken toplu dispose
 * için tasarlanmıştır — kendi kendini dispose eden bir nesne (ör. kullanıcı
 * sekmeyi kapattığında kendini dispose eden bir WebviewPanel) her açılışta
 * bu diziye eklenip hiç çıkarılmazsa, uzun bir oturumda dispose edilmiş
 * referanslar (ve onların yakaladığı closure'lar) yığılır.
 *
 * Diziden `splice` ile fiilen ÇIKARMAK yerine ilgili konumu zararsız, boş
 * bir disposable ile DEĞİŞTİRİYORUZ: VS Code, uzantı deactivate olurken
 * context.subscriptions üzerinde gezinirken (iterate ederken), bir öğenin
 * dispose() çağrısı — bu callback'i tetikleyerek — aynı diziyi splice ile
 * küçültseydi, sonraki elemanların indeksi kayar ve gezinme henüz dispose
 * edilmemiş bir sonraki öğeyi atlayabilirdi. Yerinde değiştirme dizinin
 * uzunluğunu/sırasını hiç etkilemez; asıl bellek maliyetini oluşturan
 * nesne (panel, repository ve onların closure'ları) yine de serbest
 * bırakılabilir hale gelir. Çağıran taraf, nesne kendi kendini dispose
 * ettiğinde (ör. onDidDispose içinde) döndürülen fonksiyonu çağırır; nesneyi
 * kendisi henüz dispose ETMEMİŞSE bunu da yapmak istiyorsa
 * disposable.dispose()'u ayrıca çağırmalıdır (bkz. extension.ts'teki
 * DeltaXRepository kullanımı).
 */
export function trackDisposable(context: vscode.ExtensionContext, disposable: vscode.Disposable): () => void {
  context.subscriptions.push(disposable);
  return () => {
    const idx = context.subscriptions.indexOf(disposable);
    if (idx >= 0) {
      context.subscriptions[idx] = NOOP_DISPOSABLE;
    }
  };
}
