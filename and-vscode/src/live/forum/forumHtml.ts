// Canlı Panel'in Forum sekmesi: stil, HTML ve webview betiği. panelHtml.ts
// bunları kendi sayfasına yerleştirir; betik oradaki yardımcıları (esc, hue,
// tf, th, kat, showBanner, onEnter, vscode, DIL, KATEGORI, isModerator,
// window.__identity) kullanır. Uzantı tarafı: forumDenetci.ts.
//
// Betik bir TypeScript şablon dizesinin içindedir: içinde ters bölü (\)
// kullanılmaz (şablon onu yutar), satır sonu gerektiğinde karakter koduyla
// denetlenir.

/** Forum sekmesinin stilleri. */
export function forumCss(): string {
  return `
  #tab-forum { max-width: 1180px; }
  .forum { display: grid; grid-template-columns: minmax(230px, 330px) minmax(0, 1fr); gap: 24px; align-items: start; }
  .forum-sol .composer select { flex: 1; min-width: 0; }
  .konular { list-style: none; margin: 10px 0 0; padding: 0; max-height: 72vh; overflow-y: auto; border-top: 1px solid var(--line); }
  .konular:empty::before { content: attr(data-empty); display: block; padding: 8px 0; color: var(--muted); }
  .konu-oge { padding: 8px 8px 8px 6px; border-bottom: 1px solid var(--line); border-left: 3px solid transparent; cursor: pointer; }
  .konu-oge:hover { background: var(--vscode-list-hoverBackground); }
  .konu-oge.sel { background: var(--vscode-list-inactiveSelectionBackground, var(--vscode-list-hoverBackground)); border-left-color: var(--and-accent); }
  .konu-oge:focus-visible { outline: 1px solid var(--vscode-focusBorder); outline-offset: -1px; }
  .konu-baslik { font-weight: 600; overflow-wrap: anywhere; }
  .konu-alt { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 8px; margin-top: 2px; font-size: 12px; color: var(--muted); }
  .konu-alt time { margin-left: auto; }
  .rozet { display: inline-block; font-size: 11px; line-height: 16px; padding: 0 6px; border-radius: 8px; border: 1px solid var(--and-badge); color: var(--and-badge); }
  .who + .rozet { margin-left: 6px; }
  .forum-sag { min-width: 0; }
  .forum-sag .post { margin-top: 0; }
  .post-ust { display: flex; align-items: flex-start; gap: 10px; }
  .post-ust h2 { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .post time { font-size: 12px; color: var(--muted); }
  .reply-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 8px; }
  pre.kod { position: relative; margin: 8px 0; padding: 10px 12px; border-radius: 4px; overflow-x: auto; white-space: pre; background: var(--vscode-textCodeBlock-background, rgba(128,128,128,0.14)); font-family: var(--vscode-editor-font-family); font-size: 12px; }
  .kod-dil { position: absolute; top: 2px; right: 8px; font-size: 10px; color: var(--muted); }
  .yanit-kutusu { display: flex; gap: 8px; align-items: flex-end; margin-top: 10px; }
  .yanit-kutusu textarea { min-height: 38px; }
  .profil-ust { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .profil-ust h2 { margin: 0; }
  .yasak { margin: 10px 0; padding: 8px 12px; border-left: 3px solid var(--vscode-inputValidation-errorBorder, #be1100); border-radius: 4px; background: var(--vscode-inputValidation-errorBackground); }
  .etiketler { display: flex; flex-wrap: wrap; gap: 6px; }
  .geri { margin-bottom: 10px; }
  @media (max-width: 760px) {
    .forum { grid-template-columns: 1fr; gap: 14px; }
    .konular { max-height: 32vh; }
  }`;
}

/** Forum sekmesinin HTML'i; h, HTML'e yazılan çeviridir. */
export function forumBolumu(h: (kaynak: string) => string): string {
  return `
    <section id="tab-forum">
      <div class="sec-head"><h1>${h("Forum")}</h1><p class="lede">${h("Konular yayınlandığı anda ağa yayılır ve 5 gün sonra silinir. Liste canlı güncellenir.")}</p></div>
      <div class="forum">
        <div class="forum-sol">
          <div class="composer">
            <select id="forumKategori" aria-label="${h("Kategori")}"></select>
            <button class="btn primary sm" id="forumYeniBtn">${h("Yeni konu")}</button>
          </div>
          <ul id="forumKonular" class="konular" data-empty="${h("Bu kategoride konu yok.")}"></ul>
          <div id="forumEski" style="display:none">
            <p class="muted">${h("Bu AND sürümü konu listesini vermiyor. Listeyi görmek için AND'ı güncelle; ID'sini bildiğin bir konuyu yine açabilirsin.")}</p>
            <div class="composer">
              <input type="text" id="viewPostId" class="mono" placeholder="${h("Konu ID")}" />
              <button class="btn secondary" id="viewPostBtn">${h("Göster")}</button>
            </div>
          </div>
        </div>
        <div class="forum-sag" id="forumSag"><p class="muted">${h("Okumak için soldan bir konu seç ya da yeni bir konu aç.")}</p></div>
      </div>
    </section>`;
}

/** Forum sekmesinin betiği; j, betiğe dize değişmezi olarak gömülen çeviridir. */
export function forumBetigi(j: (kaynak: string) => string): string {
  return `
  // ─── Forum ───
  let forumKonular = [];
  let forumKategoriler = [];
  let forumListe = true;
  let forumKategori = '';
  // Sağ bölmede ne var: bos | konu | yeni | profil | gitti
  let forumGorunum = 'bos';
  let forumDetay = null;
  let forumProfil = null;
  // Profil bir konudan açıldıysa o konu ("Konuya dön").
  let forumProfildenOnce = null;
  // Konu başına yazılıp gönderilmemiş yanıtlar ve yeni konu taslağı (panel açık kaldıkça).
  const forumYanitTaslagi = {};
  let forumTaslak = { category: '', title: '', body: '' };

  function benimAnahtarim() { return String((window.__identity && window.__identity.pub_key) || '').toLowerCase(); }
  function kendimMi(key) { const b = benimAnahtarim(); return !!b && String(key || '').toLowerCase() === b; }
  function rolRozeti(rol) {
    if (rol === 'founder') return '<span class="rozet">' + esc(${j("Kurucu")}) + '</span>';
    if (rol === 'moderator') return '<span class="rozet">' + esc(${j("Moderatör")}) + '</span>';
    return '';
  }
  function yazarBag(ad, key) {
    return '<span class="who title-link" tabindex="0" style="--h:' + hue(key || ad) + '" data-profil="' + esc(key || '') + '">' + esc(ad || ${j("adsız")}) + '</span>';
  }
  function tamZaman(ts) { const d = new Date(ts); return isNaN(d) ? '' : d.toLocaleString(DIL); }
  function goreli(ts) {
    const ms = Date.parse(ts);
    if (isNaN(ms)) return '';
    const fark = (ms - Date.now()) / 1000;
    const a = Math.abs(fark);
    const rtf = new Intl.RelativeTimeFormat(DIL, { numeric: 'auto' });
    if (a < 60) return rtf.format(Math.round(fark), 'second');
    if (a < 3600) return rtf.format(Math.round(fark / 60), 'minute');
    if (a < 86400) return rtf.format(Math.round(fark / 3600), 'hour');
    if (a < 86400 * 30) return rtf.format(Math.round(fark / 86400), 'day');
    return new Date(ms).toLocaleDateString(DIL);
  }
  function zamanEtiketi(ts) {
    const g = goreli(ts);
    return g ? '<time title="' + esc(tamZaman(ts)) + '">' + esc(g) + '</time>' : '';
  }
  // govdeCiz: düz metin kaçırılır; [code] … [/code] ve [code=dil] … [/code] blokları kod olarak çizilir.
  function govdeCiz(s) {
    s = String(s || '');
    let out = '';
    let i = 0;
    while (i < s.length) {
      const bas = s.indexOf('[code', i);
      if (bas < 0) break;
      const kapa = s.indexOf(']', bas);
      const etiket = kapa < 0 ? '' : s.slice(bas, kapa + 1);
      const gecerli = etiket === '[code]' || (etiket.indexOf('[code=') === 0 && etiket.length <= 27 && etiket.indexOf(' ') < 0);
      const son = gecerli ? s.indexOf('[/code]', kapa + 1) : -1;
      if (son < 0) { out += esc(s.slice(i, bas + 5)); i = bas + 5; continue; }
      // Kod bloğu zaten blok: hemen önündeki ve arkasındaki satır sonu boş satır bırakmasın.
      let onceki = s.slice(i, bas);
      if (onceki.charCodeAt(onceki.length - 1) === 10) onceki = onceki.slice(0, onceki.charCodeAt(onceki.length - 2) === 13 ? -2 : -1);
      out += esc(onceki);
      const dil = etiket === '[code]' ? '' : etiket.slice(6, -1);
      let kod = s.slice(kapa + 1, son);
      if (kod.charCodeAt(0) === 13 && kod.charCodeAt(1) === 10) kod = kod.slice(2);
      else if (kod.charCodeAt(0) === 10) kod = kod.slice(1);
      out += '<pre class="kod">' + (dil ? '<span class="kod-dil">' + esc(dil) + '</span>' : '') + '<code>' + esc(kod) + '</code></pre>';
      i = son + 7;
      if (s.charCodeAt(i) === 13 && s.charCodeAt(i + 1) === 10) i += 2;
      else if (s.charCodeAt(i) === 10) i += 1;
    }
    return out + esc(s.slice(i));
  }
  function profilBaglari(kok) {
    kok.querySelectorAll('[data-profil]').forEach(s => {
      const ac = () => profilAc(s.dataset.profil);
      s.addEventListener('click', ac);
      s.addEventListener('keydown', (e) => { if (e.key === 'Enter') ac(); });
    });
  }
  function profilAc(key) { if (key) vscode.postMessage({ type: 'profile', key }); }
  function forumAc(id) { if (id) vscode.postMessage({ type: 'forumOpen', id }); }

  function forumKategorileriCiz() {
    const sec = document.getElementById('forumKategori');
    const kats = forumKategoriler.slice();
    if (forumKategori && !kats.some(k => k.kategori === forumKategori)) kats.push({ kategori: forumKategori, sayi: 0 });
    sec.innerHTML = '<option value="">' + esc(tf(${j("Tüm kategoriler (%d)")}, forumKonular.length)) + '</option>' +
      kats.map(k => '<option value="' + esc(k.kategori) + '">' + esc(kat(k.kategori)) + ' (' + k.sayi + ')</option>').join('');
    sec.value = forumKategori;
  }

  function forumListesiCiz() {
    document.getElementById('forumKategori').style.display = forumListe ? '' : 'none';
    document.getElementById('forumKonular').style.display = forumListe ? '' : 'none';
    document.getElementById('forumEski').style.display = forumListe ? 'none' : '';
    if (!forumListe) return;
    forumKategorileriCiz();
    const ul = document.getElementById('forumKonular');
    const secili = forumGorunum === 'konu' && forumDetay ? forumDetay.id : '';
    ul.innerHTML = forumKonular.filter(k => !forumKategori || k.category === forumKategori).map(k =>
      '<li class="konu-oge' + (k.id === secili ? ' sel' : '') + '" tabindex="0" data-konu-ac="' + esc(k.id) + '">' +
      '<div class="konu-baslik">' + esc(k.title) + '</div>' +
      '<div class="konu-alt"><span class="who" style="--h:' + hue(k.author_key || k.author_name) + '">' + esc(k.author_name) + '</span>' + rolRozeti(k.author_role) +
      (forumKategori ? '' : '<span>' + esc(kat(k.category)) + '</span>') +
      // AND yanıtsız konuda reply_count'u göndermez; zamanı olan (yeni AND'ın) konuda bu 0 yanıttır.
      (k.created_at || typeof k.reply_count === 'number' ? '<span>' + esc(tf(${j("%d yanıt")}, k.reply_count || 0)) + '</span>' : '') +
      zamanEtiketi(k.last_activity || k.created_at) + '</div></li>'
    ).join('');
    ul.querySelectorAll('[data-konu-ac]').forEach(li => {
      li.addEventListener('click', () => forumAc(li.dataset.konuAc));
      li.addEventListener('keydown', (e) => { if (e.key === 'Enter') forumAc(li.dataset.konuAc); });
    });
  }

  function forumSagCiz() {
    const sag = document.getElementById('forumSag');
    if (forumGorunum === 'konu' && forumDetay) return konuCiz(sag);
    if (forumGorunum === 'yeni') return yeniKonuCiz(sag);
    if (forumGorunum === 'profil' && forumProfil) return profilCiz(sag);
    if (forumGorunum === 'gitti') { sag.innerHTML = '<p class="muted">' + esc(${j("Bu konu artık yok. Silinmiş ya da süresi dolmuş olabilir.")}) + '</p>'; return; }
    sag.innerHTML = '<p class="muted">' + esc(${j("Okumak için soldan bir konu seç ya da yeni bir konu aç.")}) + '</p>';
  }

  function konuCiz(sag) {
    const d = forumDetay;
    const yanitlar = d.replies || [];
    const silinebilir = kendimMi(d.author_key) || isModerator;
    sag.innerHTML = '<article class="post"><div class="post-ust"><h2>' + esc(d.title) + '</h2>' +
      (silinebilir ? '<button class="btn ghost sm danger" data-sil="' + esc(d.id) + '">' + esc(${j("Sil")}) + '</button>' : '') + '</div>' +
      '<p class="muted">' + th(${j("%s yazdı, %s kategorisinde.")}, yazarBag(d.author_name, d.author_key) + rolRozeti(d.author_role), esc(kat(d.category))) + ' ' + zamanEtiketi(d.created_at) + '</p>' +
      '<div class="body">' + govdeCiz(d.body) + '</div>' +
      '<h3>' + esc(yanitlar.length ? tf(${j("%d yanıt")}, yanitlar.length) : ${j("Henüz yanıt yok")}) + '</h3>' +
      '<ul class="list replies">' + yanitlar.map(r =>
        '<li><div class="grow"><div class="reply-meta">' + yazarBag(r.author_name, r.author_key) + rolRozeti(r.author_role) + zamanEtiketi(r.created_at) + '</div>' +
        '<div class="body">' + govdeCiz(r.body) + '</div></div></li>').join('') + '</ul>' +
      '<div class="yanit-kutusu"><textarea id="replyBody" rows="2" placeholder="' + esc(${j("Yanıtını yaz…")}) + '"></textarea>' +
      '<button class="btn primary" id="replySend">' + esc(${j("Yanıtla")}) + '</button></div></article>';
    const kutu = document.getElementById('replyBody');
    kutu.value = forumYanitTaslagi[d.id] || '';
    kutu.addEventListener('input', () => { if (forumDetay) forumYanitTaslagi[forumDetay.id] = kutu.value; });
    document.getElementById('replySend').addEventListener('click', () => {
      if (!forumDetay || !kutu.value.trim()) return;
      forumYanitTaslagi[forumDetay.id] = kutu.value;
      vscode.postMessage({ type: 'reply', postId: forumDetay.id, body: kutu.value });
    });
    onEnter('replyBody', 'replySend');
    sag.querySelectorAll('[data-sil]').forEach(b => b.addEventListener('click', () =>
      vscode.postMessage({ type: 'deletePost', postId: d.id, own: kendimMi(d.author_key), title: d.title })));
    profilBaglari(sag);
  }

  function yeniKonuCiz(sag) {
    const secenekler = Object.keys(KATEGORI).map(c => '<option value="' + esc(c) + '">' + esc(kat(c)) + '</option>').join('');
    sag.innerHTML = '<div class="panel"><h2 style="margin-top:0">' + esc(${j("Yeni konu")}) + '</h2>' +
      '<div class="field"><label for="postCategory">' + esc(${j("Kategori")}) + '</label><select id="postCategory">' + secenekler + '</select></div>' +
      '<div class="field"><label for="postTitle">' + esc(${j("Başlık")}) + '</label><input type="text" id="postTitle" /></div>' +
      '<div class="field"><label for="postBody">' + esc(${j("İçerik")}) + '</label><textarea id="postBody"></textarea></div>' +
      '<p class="muted">' + esc(${j("Kod için [code] … [/code] ya da [code=python] … [/code] kullan.")}) + '</p>' +
      '<div class="composer"><button class="btn primary" id="postCreate">' + esc(${j("Konuyu yayınla")}) + '</button>' +
      '<button class="btn ghost" id="postCancel">' + esc(${j("Vazgeç")}) + '</button></div></div>';
    const sec = document.getElementById('postCategory');
    const baslik = document.getElementById('postTitle');
    const govde = document.getElementById('postBody');
    sec.value = forumTaslak.category || forumKategori || 'Genel';
    baslik.value = forumTaslak.title;
    govde.value = forumTaslak.body;
    const kaydet = () => { forumTaslak = { category: sec.value, title: baslik.value, body: govde.value }; };
    [sec, baslik, govde].forEach(e => { e.addEventListener('input', kaydet); e.addEventListener('change', kaydet); });
    document.getElementById('postCreate').addEventListener('click', () => {
      kaydet();
      const title = baslik.value.trim();
      const body = govde.value.trim();
      if (!title || !body) { showBanner(${j("Başlık ve içerik boş olamaz.")}, false); return; }
      vscode.postMessage({ type: 'createPost', category: sec.value, title, body });
    });
    document.getElementById('postCancel').addEventListener('click', () => {
      forumTaslak = { category: '', title: '', body: '' };
      forumGorunum = forumDetay ? 'konu' : 'bos';
      forumSagCiz();
    });
  }

  function profilCiz(sag) {
    const p = forumProfil;
    const ad = p.name || ${j("adsız")};
    let html = (forumProfildenOnce ? '<button class="btn ghost sm geri" id="profilGeri">' + esc(${j("← Konuya dön")}) + '</button>' : '') +
      '<article class="post"><div class="profil-ust"><h2><span class="who" style="--h:' + hue(p.key) + '">' + esc(ad) + '</span></h2>' + rolRozeti(p.role) + '</div>' +
      '<p class="mono muted">' + esc(p.key) + '</p>' +
      '<p>' + esc(tf(${j("%d konu")}, p.post_count || 0)) + ', ' + esc(tf(${j("%d yanıt")}, p.reply_count || 0)) +
      (p.first_seen ? '. ' + esc(tf(${j("İlk görülme: %s")}, new Date(p.first_seen).toLocaleDateString(DIL))) : '') + '</p>';
    if (p.ban) {
      html += '<div class="yasak">' + esc(p.ban.permanent ? ${j("Kalıcı olarak yasaklı.")} : tf(${j("%s tarihine kadar yasaklı.")}, tamZaman(p.ban.expires_at))) +
        (p.ban.reason ? '<br>' + esc(tf(${j("Sebep: %s")}, p.ban.reason)) : '') + '</div>';
    }
    if (p.status) html += '<p><em>' + esc(p.status) + '</em></p>';
    if (p.bio) html += '<div class="body">' + esc(p.bio) + '</div>';
    if (p.tags && p.tags.length) html += '<h3>' + esc(${j("İlgi alanları")}) + '</h3><div class="etiketler">' + p.tags.map(e => '<span class="rozet">' + esc(kat(e)) + '</span>').join('') + '</div>';
    const baglar = [];
    if (p.link) baglar.push(esc(p.link));
    if (p.github_user) baglar.push('GitHub: ' + esc(p.github_user));
    if (baglar.length) html += '<p class="muted">' + baglar.join('<br>') + '</p>';
    if (p.site_pages && p.site_pages.length) {
      html += '<h3>' + esc(${j("Site sayfaları")}) + '</h3><ul class="list">' + p.site_pages.map(s =>
        '<li><div class="grow"><strong>' + esc(s.title || s.slug) + '</strong> <span class="mono muted">/' + esc(s.slug) + '</span></div></li>').join('') + '</ul>';
    }
    const yasaklanabilir = isModerator && !kendimMi(p.key);
    if (yasaklanabilir) {
      const sureler = [['3600', ${j("1 saat")}], ['86400', ${j("1 gün")}], ['604800', ${j("7 gün")}], ['2592000', ${j("30 gün")}], ['kalici', ${j("Kalıcı")}]];
      html += '<h3>' + esc(${j("Yazarı yasakla")}) + '</h3><div class="panel">' +
        '<div class="field"><label for="banSure">' + esc(${j("Süre")}) + '</label><select id="banSure">' +
        sureler.map(s => '<option value="' + s[0] + '">' + esc(s[1]) + '</option>').join('') + '</select></div>' +
        '<div class="field"><label for="banSebep">' + esc(${j("Sebep")}) + '</label><input type="text" id="banSebep" /></div>' +
        '<button class="btn primary" id="banGonder">' + esc(${j("Yasakla")}) + '</button></div>';
    }
    sag.innerHTML = html + '</article>';
    if (forumProfildenOnce) {
      document.getElementById('profilGeri').addEventListener('click', () => {
        forumDetay = forumProfildenOnce;
        forumProfildenOnce = null;
        forumGorunum = 'konu';
        forumSagCiz();
      });
    }
    if (yasaklanabilir) {
      document.getElementById('banGonder').addEventListener('click', () => {
        const sebep = document.getElementById('banSebep').value.trim();
        if (!sebep) { showBanner(${j("Sebep boş olamaz.")}, false); return; }
        const sure = document.getElementById('banSure').value;
        const m = { type: 'ban', key: p.key, name: ad };
        if (sure === 'kalici') m.permanent = true; else m.seconds = Number(sure);
        m.reason = sebep;
        vscode.postMessage(m);
      });
    }
  }

  document.getElementById('forumKategori').addEventListener('change', () => {
    forumKategori = document.getElementById('forumKategori').value;
    forumListesiCiz();
  });
  document.getElementById('forumYeniBtn').addEventListener('click', () => {
    forumGorunum = 'yeni';
    forumSagCiz();
    forumListesiCiz();
  });
  document.getElementById('viewPostBtn').addEventListener('click', () => forumAc(document.getElementById('viewPostId').value.trim()));
  onEnter('viewPostId', 'viewPostBtn');

  // forumMesaji: uzantıdan gelen forum iletilerini işler; forum iletisi değilse false.
  function forumMesaji(msg) {
    switch (msg.type) {
      case 'forumPosts':
        forumKonular = msg.konular || [];
        forumKategoriler = msg.kategoriler || [];
        forumListe = msg.liste !== false;
        forumListesiCiz();
        return true;
      case 'postDetail': {
        const d = msg.data;
        if (msg.ac) {
          forumDetay = d;
          forumGorunum = 'konu';
          forumSagCiz();
          forumListesiCiz();
          return true;
        }
        // Canlı yenileme: yalnızca o konu açıksa yeniden çizilir; başka ekrandan atmaz.
        if (forumDetay && forumDetay.id === d.id) {
          forumDetay = d;
          if (forumGorunum === 'konu') forumSagCiz();
        }
        if (forumProfildenOnce && forumProfildenOnce.id === d.id) forumProfildenOnce = d;
        return true;
      }
      case 'postGone':
        if (msg.ac || (forumDetay && forumDetay.id === msg.id)) {
          if (forumDetay && forumDetay.id === msg.id) forumDetay = null;
          if (msg.ac || forumGorunum === 'konu') { forumGorunum = 'gitti'; forumSagCiz(); }
        }
        return true;
      case 'postDeleted':
        delete forumYanitTaslagi[msg.id];
        if (forumDetay && forumDetay.id === msg.id) {
          forumDetay = null;
          if (forumGorunum === 'konu') { forumGorunum = 'bos'; forumSagCiz(); }
        }
        return true;
      case 'replySent':
        delete forumYanitTaslagi[msg.postId];
        if (forumGorunum === 'konu' && forumDetay && forumDetay.id === msg.postId) document.getElementById('replyBody').value = '';
        return true;
      case 'forumCreated':
        forumTaslak = { category: '', title: '', body: '' };
        return true;
      case 'profile':
        // Konudan açıldıysa "Konuya dön" o konuya döner; profil yenilenirken (yasaklamadan sonra) korunur.
        if (forumGorunum === 'konu') forumProfildenOnce = forumDetay;
        else if (forumGorunum !== 'profil') forumProfildenOnce = null;
        forumProfil = msg.data;
        forumGorunum = 'profil';
        forumSagCiz();
        return true;
    }
    return false;
  }
`;
}
