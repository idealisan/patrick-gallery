/* Immich (Go port) — web UI
 * Vanilla JS SPA, no build step. Talks to the Immich-compatible Go backend.
 * Embedded and served by internal/webroot. */
(function () {
  'use strict';

  // ----------------------------------------------------------------- state
  const token0 = localStorage.getItem('immich_token');
  const App = {
    token: token0,
    user: null,
    view: 'photos',
    params: {},
    viewerList: [],
    viewerIndex: -1,
    selected: new Set(),
    thumbCache: new Map(), // assetId -> object URL (cached across views)
    loading: false,
  };

  const $ = (id) => document.getElementById(id);

  // --------------------------------------------------------------- toast
  let toastTimer = null;
  function toast(msg, ok = true) {
    const t = $('toast');
    t.textContent = msg;
    t.className = 'toast show' + (ok ? '' : ' err');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { t.className = 'toast'; }, 2200);
  }

  // ----------------------------------------------------------------- api
  function authHeaders(json) {
    const h = {};
    if (App.token) h['Authorization'] = 'Bearer ' + App.token;
    if (json) h['Content-Type'] = 'application/json';
    return h;
  }
  async function api(method, path, body, isJson) {
    const opts = { method, headers: authHeaders(!!isJson) };
    if (body !== undefined) opts.body = isJson ? JSON.stringify(body) : body;
    const res = await fetch(path, opts);
    if (res.status === 401 && App.token) {
      // session expired
      localStorage.removeItem('immich_token');
      App.token = null;
      showLogin();
      throw new Error('unauthorized');
    }
    let data = null;
    try { data = await res.json(); } catch (_) { /* non-json */ }
    return { status: res.status, body: data };
  }
  const getJSON = (p) => api('GET', p);
  const delJSON = (p, b) => api('DELETE', p, b, true);
  const putJSON = (p, b) => api('PUT', p, b, true);
  const postJSON = (p, b) => api('POST', p, b, true);

  function previewURL(id) { return '/api/assets/' + id + '/preview'; }
  function origURL(id) { return '/api/assets/' + id + '/original'; }
  function encURL(id) { return '/api/assets/' + id + '/encoded-video/' + Date.now(); }

  // fetch a thumbnail as a cached object URL (auth-protected endpoint)
  async function thumbURL(id) {
    if (App.thumbCache.has(id)) return App.thumbCache.get(id);
    try {
      const r = await fetch('/api/assets/' + id + '/thumbnail/' + Date.now(), { headers: authHeaders(false) });
      if (!r.ok) return null;
      const blob = await r.blob();
      const url = URL.createObjectURL(blob);
      App.thumbCache.set(id, url);
      return url;
    } catch (_) { return null; }
  }

  // ------------------------------------------------- lazy thumbnail loader
  const thumbObserver = ('IntersectionObserver' in window)
    ? new IntersectionObserver((entries, obs) => {
        entries.forEach(async (e) => {
          if (!e.isIntersecting) return;
          const img = e.target;
          obs.unobserve(img);
          const url = await thumbURL(img.dataset.id);
          if (url) img.src = url;
        });
      }, { rootMargin: '300px' })
    : null;

  // --------------------------------------------------------------- auth
  async function bootstrap() {
    if (!App.token) { showLogin(); return; }
    const r = await getJSON('/api/users/me');
    if (r.status === 200 && r.body) {
      App.user = r.body;
      showApp();
    } else {
      localStorage.removeItem('immich_token');
      App.token = null;
      showLogin();
    }
  }

  function showLogin() {
    $('login-screen').hidden = false;
    $('app').hidden = true;
  }
  function showApp() {
    $('login-screen').hidden = true;
    $('app').hidden = false;
    if (App.user) {
      $('user-name').textContent = App.user.name || App.user.email || 'user';
      const init = (App.user.name || App.user.email || '?').charAt(0).toUpperCase();
      $('user-avatar').textContent = init;
    }
    route();
  }

  // --------------------------------------------------------------- router
  function parseHash() {
    const h = location.hash.replace(/^#\/?/, '');
    const parts = h.split('/').filter(Boolean);
    App.view = parts[0] || 'photos';
    App.params = {};
    if (App.view === 'albums' && parts[1]) App.params.albumId = parts[1];
  }
  function route() {
    parseHash();
    // sidebar active state
    document.querySelectorAll('.nav-item').forEach((n) => {
      n.classList.toggle('active', n.dataset.view === App.view);
    });
    // global search box only on photos/search
    $('global-search').hidden = !(App.view === 'photos' || App.view === 'search');
    App.selected.clear();
    renderBulkBar();

    const titles = {
      photos: 'Photos', albums: 'Albums', search: 'Search',
      favorites: 'Favorites', map: 'Map', archive: 'Archive', trash: 'Trash', admin: 'Admin',
    };
    $('page-title').textContent = titles[App.view] || 'Photos';

    const view = $('view');
    view.innerHTML = '';
    switch (App.view) {
      case 'photos': return viewPhotos(view);
      case 'albums': return App.params.albumId ? viewAlbumDetail(view, App.params.albumId) : viewAlbums(view);
      case 'search': return viewSearch(view);
      case 'favorites': return viewAssetList(view, { isFavorite: 'true' }, 'No favorites yet');
      case 'map': return viewMap(view);
      case 'archive': return viewAssetList(view, { isArchived: 'true' }, 'Archive is empty');
      case 'trash': return viewTrash(view);
      case 'admin': return viewAdmin(view);
      default: return viewPhotos(view);
    }
  }

  // ------------------------------------------------------- timeline/photos
  const bucketObserver = ('IntersectionObserver' in window)
    ? new IntersectionObserver((entries, obs) => {
        entries.forEach((e) => {
          if (!e.isIntersecting) return;
          obs.unobserve(e.target);
          const load = e.target._load;
          if (load) load();
        });
      }, { rootMargin: '400px' })
    : null;

  async function viewPhotos(view) {
    const r = await getJSON('/api/timeline/buckets?size=MONTH');
    if (!r.body || !Array.isArray(r.body)) { view.innerHTML = '<div class="empty">Failed to load timeline.</div>'; return; }
    if (r.body.length === 0) { view.innerHTML = '<div class="empty">No photos yet — upload some above.</div>'; return; }

    // buckets come oldest-first; show newest first
    const buckets = r.body.slice().reverse();
    App.viewerList = [];

    // "On This Day" memories — rendered at the very top, only if present.
    // Degrades gracefully (nothing shown) when the endpoint is absent/empty.
    renderMemories(view);

    for (const b of buckets) {
      const label = document.createElement('div');
      label.className = 'bucket-label';
      label.textContent = formatBucket(b.timeBucket) + '  ·  ' + b.count;
      view.appendChild(label);

      const grid = document.createElement('div');
      grid.className = 'grid';
      view.appendChild(grid);

      // lazy-load this bucket's assets when scrolled into view
      const load = async () => {
        grid.innerHTML = '<div class="spinner">Loading…</div>';
        const br = await getJSON('/api/timeline/bucket?timeBucket=' + encodeURIComponent(b.timeBucket));
        grid.innerHTML = '';
        const assets = (br.body && br.body.assets) || [];
        for (const a of assets) App.viewerList.push(a);
        if (assets.length === 0) { grid.innerHTML = '<div class="muted">No assets.</div>'; return; }
        renderTiles(grid, assets, App.viewerList);
      };
      if (bucketObserver) { grid._load = load; bucketObserver.observe(label); }
      else { await load(); }
    }
  }

  // --------------------------------------------------------- memories
  // Fetches "On This Day" memories and renders them at the top of the Photos
  // view. Each group shows the year(s) and a horizontal strip of thumbnails.
  // Reuses the shared thumbnail loader/observer. Renders nothing if the
  // endpoint is missing, errors, or returns an empty list (graceful).
  async function renderMemories(container) {
    try {
      const now = new Date();
      const mm = String(now.getMonth() + 1).padStart(2, '0');
      const dd = String(now.getDate()).padStart(2, '0');
      const r = await getJSON('/api/memories?day=' + mm + '-' + dd);
      const groups = r.body;
      if (!Array.isArray(groups) || groups.length === 0) return; // nothing to show

      const section = document.createElement('div');
      section.className = 'memories';
      const title = document.createElement('div');
      title.className = 'memories-title';
      title.textContent = '🕒  On This Day';
      section.appendChild(title);

      let anyTiles = false;
      for (const g of groups) {
        const gEl = document.createElement('div');
        gEl.className = 'memories-group';
        const years = Array.isArray(g.years) ? g.years.join(', ') : '';
        if (years) {
          const y = document.createElement('div');
          y.className = 'memories-year';
          y.textContent = years;
          gEl.appendChild(y);
        }
        const strip = document.createElement('div');
        strip.className = 'memories-strip';
        const assets = Array.isArray(g.assets) ? g.assets : [];
        for (const a of assets) {
          const t = document.createElement('div');
          t.className = 'tile';
          const img = document.createElement('img');
          img.dataset.id = a.id;
          img.alt = a.originalFileName || a.id;
          t.appendChild(img);
          if (a.hasThumbnail) {
            if (thumbObserver) thumbObserver.observe(img);
            else (async () => { img.src = await thumbURL(a.id); })();
          }
          if (a.type === 'VIDEO') {
            const p = document.createElement('div'); p.className = 'play'; p.textContent = '▶';
            t.appendChild(p);
          }
          if (a.livePhotoVideoId) {
            const lp = document.createElement('div'); lp.className = 'live'; lp.textContent = '●';
            t.appendChild(lp);
          }
          t.onclick = () => openViewer([a], 0);
          strip.appendChild(t);
          anyTiles = true;
        }
        gEl.appendChild(strip);
        section.appendChild(gEl);
      }

      if (!anyTiles) return; // groups present but no viewable assets
      // keep the section at the top even if buckets already rendered
      container.insertBefore(section, container.firstChild);
    } catch (_) {
      // graceful: never break the timeline if memories are unavailable
    }
  }

  // ------------------------------------------------------------ asset list
  async function viewAssetList(view, query, emptyMsg) {
    const qs = new URLSearchParams(Object.assign({ take: 200 }, query)).toString();
    const r = await getJSON('/api/assets?' + qs);
    const assets = (r.body && r.body.assets) || [];
    App.viewerList = assets.slice();
    if (assets.length === 0) { view.innerHTML = '<div class="empty">' + emptyMsg + '</div>'; return; }
    renderTiles(view, assets, App.viewerList);
  }

  async function viewTrash(view) {
    const r = await getJSON('/api/assets?isTrash=true&take=200');
    const assets = (r.body && r.body.assets) || [];
    App.viewerList = assets.slice();
    if (assets.length === 0) { view.innerHTML = '<div class="empty">Trash is empty</div>'; return; }
    const bar = document.createElement('div');
    bar.className = 'toolbar';
    const empty = document.createElement('button');
    empty.className = 'btn-ghost'; empty.textContent = 'Empty trash';
    empty.onclick = async () => {
      if (!confirm('Permanently delete all items in trash?')) return;
      await delJSON('/api/assets', { ids: assets.map((a) => a.id), force: true });
      toast('Trash emptied'); route();
    };
    bar.appendChild(empty);
    view.appendChild(bar);
    renderTiles(view, assets, App.viewerList);
  }

  // --------------------------------------------------------------- map
  async function viewMap(view) {
    const r = await getJSON('/api/map/markers');
    const markers = (r.body && r.body.markers) || [];
    if (markers.length === 0) {
      view.innerHTML = '<div class="empty">No geo-tagged photos yet. Photos with GPS EXIF will be plotted on the map here.</div>';
      return;
    }

    const wrap = document.createElement('div');
    wrap.className = 'map-wrap';

    const left = document.createElement('div');
    left.className = 'map-canvas-wrap';

    const canvas = document.createElement('canvas');
    canvas.width = 1000; canvas.height = 500;
    canvas.className = 'map-canvas';
    left.appendChild(canvas);

    const hint = document.createElement('div');
    hint.className = 'muted map-hint';
    hint.textContent = markers.length + ' location' + (markers.length === 1 ? '' : 's') + ' · click a point or a list item to view';
    left.appendChild(hint);
    wrap.appendChild(left);

    const list = document.createElement('div');
    list.className = 'map-list';
    wrap.appendChild(list);

    view.appendChild(wrap);

    const ctx = canvas.getContext('2d');
    drawMapBg(ctx, canvas.width, canvas.height);

    // representative asset list so clicking a marker opens the lightbox
    const assetList = [];
    const proj = (lat, lon) => ({
      x: (lon + 180) / 360 * canvas.width,
      y: (90 - lat) / 180 * canvas.height,
    });

    for (const m of markers) {
      const { x, y } = proj(m.lat, m.lon);
      const radius = Math.max(4, Math.min(11, 3 + Math.sqrt(m.count) * 1.6));
      ctx.beginPath();
      ctx.arc(x, y, radius, 0, 2 * Math.PI);
      ctx.fillStyle = 'rgba(220,40,60,0.78)';
      ctx.fill();
      ctx.lineWidth = 1.5; ctx.strokeStyle = '#fff'; ctx.stroke();

      assetList.push({ id: m.assetId });

      const item = document.createElement('div');
      item.className = 'map-item';
      const img = document.createElement('img');
      img.dataset.id = m.assetId;
      if (thumbObserver) thumbObserver.observe(img);
      else (async () => { img.src = await thumbURL(m.assetId); })();
      const label = document.createElement('div');
      label.className = 'map-item-label';
      const place = m.city || m.country || (m.lat.toFixed(2) + ', ' + m.lon.toFixed(2));
      label.textContent = place + (m.count > 1 ? '  (' + m.count + ')' : '');
      item.append(img, label);
      item.onclick = () => openViewer(assetList, assetList.findIndex((a) => a.id === m.assetId));
      list.appendChild(item);
    }
  }

  // draws a simple equirectangular world grid (offline, no map tiles needed)
  function drawMapBg(ctx, w, h) {
    ctx.fillStyle = '#0e1b2a';
    ctx.fillRect(0, 0, w, h);
    ctx.strokeStyle = 'rgba(255,255,255,0.08)';
    ctx.lineWidth = 1;
    for (let lon = -180; lon <= 180; lon += 30) {
      const x = (lon + 180) / 360 * w;
      ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke();
    }
    for (let lat = -90; lat <= 90; lat += 30) {
      const y = (90 - lat) / 180 * h;
      ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
    }
    // equator emphasis
    ctx.strokeStyle = 'rgba(255,255,255,0.18)';
    ctx.beginPath(); ctx.moveTo(0, h / 2); ctx.lineTo(w, h / 2); ctx.stroke();
    ctx.fillStyle = 'rgba(255,255,255,0.25)';
    ctx.font = '12px sans-serif';
    ctx.fillText('Equator', 6, h / 2 - 4);
  }

  // -------------------------------------------------------------- albums
  async function viewAlbums(view) {
    const r = await getJSON('/api/albums');
    const albums = (r.body || []);
    const head = document.createElement('div');
    head.className = 'toolbar';
    const create = document.createElement('button');
    create.className = 'btn-primary'; create.textContent = '+ New album';
    create.onclick = async () => {
      const name = prompt('Album name', 'New Album');
      if (!name) return;
      const cr = await postJSON('/api/albums', { albumName: name });
      if (cr.status === 201) { toast('Album created'); location.hash = '#/albums/' + cr.body.id; }
      else toast('Could not create album', false);
    };
    head.appendChild(create);
    view.appendChild(head);

    if (albums.length === 0) { view.innerHTML += '<div class="empty">No albums yet.</div>'; return; }
    const cards = document.createElement('div');
    cards.className = 'cards';
    for (const al of albums) {
      const card = document.createElement('div');
      card.className = 'album-card';
      const cover = document.createElement('div');
      cover.className = 'album-cover';
      if (al.albumThumbnailAssetId) {
        const img = document.createElement('img');
        img.dataset.id = al.albumThumbnailAssetId;
        cover.appendChild(img);
        if (thumbObserver) thumbObserver.observe(img); else (async () => { img.src = await thumbURL(al.albumThumbnailAssetId); })();
      } else {
        cover.textContent = '🖼';
      }
      const meta = document.createElement('div');
      meta.className = 'album-meta';
      meta.innerHTML = '<div class="name"></div><div class="count">' + (al.assetCount || 0) + ' items</div>';
      meta.querySelector('.name').textContent = al.albumName;
      card.appendChild(cover); card.appendChild(meta);
      card.onclick = () => { location.hash = '#/albums/' + al.id; };
      cards.appendChild(card);
    }
    view.appendChild(cards);
  }

  async function viewAlbumDetail(view, albumId) {
    const ar = await getJSON('/api/albums/' + albumId);
    if (ar.status !== 200 || !ar.body) { view.innerHTML = '<div class="empty">Album not found</div>'; return; }
    const album = ar.body;
    const r = await getJSON('/api/albums/' + albumId + '/assets');
    const assets = (r.body && r.body.assets) || [];
    App.viewerList = assets.slice();

    const bar = document.createElement('div');
    bar.className = 'toolbar';
    const title = document.createElement('h2');
    title.style.margin = '0'; title.textContent = album.albumName;
    const rename = document.createElement('button');
    rename.className = 'btn-ghost'; rename.textContent = 'Rename';
    rename.onclick = async () => {
      const name = prompt('Rename album', album.albumName);
      if (!name) return;
      await putJSON('/api/albums/' + albumId, { albumName: name });
      album.albumName = name; title.textContent = name; toast('Renamed');
    };
    const add = document.createElement('button');
    add.className = 'btn-ghost'; add.textContent = '+ Add assets';
    add.onclick = () => addAssetsToAlbum(albumId);
    const setCover = document.createElement('button');
    setCover.className = 'btn-ghost'; setCover.textContent = 'Set cover';
    setCover.onclick = () => pickAsset((assetId) => postJSON('/api/albums/' + albumId + '/cover', { assetId }).then(() => { toast('Cover set'); route(); }));
    const del = document.createElement('button');
    del.className = 'btn-ghost'; del.textContent = 'Delete album';
    del.onclick = async () => {
      if (!confirm('Delete album "' + album.albumName + '"?')) return;
      await delJSON('/api/albums/' + albumId);
      toast('Album deleted'); location.hash = '#/albums';
    };
    const share = document.createElement('button');
    share.className = 'btn-ghost'; share.textContent = '🔗 Share album';
    share.onclick = async () => {
      const r = await postJSON('/api/shared-links', { type: 'ALBUM', albumId: albumId });
      if (r.status === 201 && r.body && r.body.key) {
        const url = location.origin + '/share/' + r.body.key;
        try { await navigator.clipboard.writeText(url); } catch (_) {}
        toast('Share link copied to clipboard');
        window.prompt('Share link (copied):', url);
      } else toast('Could not create share', false);
    };
    bar.append(title, rename, add, setCover, share, del);
    view.appendChild(bar);

    if (assets.length === 0) { view.innerHTML += '<div class="empty">No assets in this album. Use “Add assets”.</div>'; return; }
    renderTiles(view, assets, App.viewerList, { albumId });
  }

  async function addAssetsToAlbum(albumId) {
    const r = await getJSON('/api/assets?take=100');
    const assets = (r.body && r.body.assets) || [];
    const chosen = await pickAssets(assets, 'Add to album');
    if (!chosen || chosen.length === 0) return;
    const res = await postJSON('/api/albums/' + albumId + '/assets', { ids: chosen });
    if (res.status === 200) { toast('Added ' + chosen.length + ' asset(s)'); route(); }
    else toast('Failed to add', false);
  }

  // -------------------------------------------------------------- search
  async function viewSearch(view) {
    const bar = document.createElement('div');
    bar.className = 'toolbar';
    const inp = document.createElement('input');
    inp.type = 'search'; inp.placeholder = 'Search by filename, camera, place…'; inp.style.maxWidth = '420px';
    const go = document.createElement('button');
    go.className = 'btn-primary'; go.textContent = 'Search';
    const run = async () => {
      const q = inp.value.trim();
      const container = $('search-results') || (() => { const c = document.createElement('div'); c.id = 'search-results'; view.appendChild(c); return c; })();
      if (!q) { container.innerHTML = ''; return; }
      const r = await postJSON('/api/search', { query: q, take: 200 });
      const assets = (r.body && r.body.assets) || [];
      App.viewerList = assets.slice();
      container.innerHTML = '';
      if (assets.length === 0) { container.innerHTML = '<div class="empty">No matches</div>'; return; }
      renderTiles(container, assets, App.viewerList);
    };
    inp.addEventListener('keydown', (e) => { if (e.key === 'Enter') run(); });
    go.onclick = run;
    bar.append(inp, go);
    view.appendChild(bar);
    const c = document.createElement('div'); c.id = 'search-results'; view.appendChild(c);

    // global search box in topbar also drives this view
    const gs = $('global-search');
    gs.value = '';
    gs.oninput = () => { inp.value = gs.value; run(); };
  }

  // ---------------------------------------------------------------- admin
  async function viewAdmin(view) {
    const c = await getJSON('/api/assets/count');
    const counts = (c.body || { photos: 0, videos: 0, total: 0 });
    const about = await getJSON('/api/server/about');
    const ver = (about.body && about.body.version) || '—';
    const grid = document.createElement('div');
    grid.className = 'admin-grid';
    grid.innerHTML =
      '<div class="stat-card"><div class="n">' + counts.total + '</div><div class="l">Total assets</div></div>' +
      '<div class="stat-card"><div class="n">' + counts.photos + '</div><div class="l">Photos</div></div>' +
      '<div class="stat-card"><div class="n">' + counts.videos + '</div><div class="l">Videos</div></div>' +
      '<div class="stat-card"><div class="n">' + ver + '</div><div class="l">Server version</div></div>';
    view.appendChild(grid);

    const pwd = document.createElement('div');
    pwd.className = 'stat-card'; pwd.style.marginTop = '16px';
    pwd.innerHTML = '<h4 style="margin:0 0 8px">Change password</h4>';
    const cur = document.createElement('input'); cur.type = 'password'; cur.placeholder = 'current password';
    const neu = document.createElement('input'); neu.type = 'password'; neu.placeholder = 'new password'; neu.style.marginTop = '8px';
    const btn = document.createElement('button'); btn.className = 'btn-primary'; btn.textContent = 'Update'; btn.style.marginTop = '10px';
    btn.onclick = async () => {
      const r = await postJSON('/api/auth/change-password', { password: cur.value, newPassword: neu.value });
      if (r.status === 204) { toast('Password updated'); cur.value = neu.value = ''; }
      else toast('Update failed', false);
    };
    pwd.append(cur, neu, btn);
    view.appendChild(pwd);

    // ---- libraries (disk scan) ----
    const libWrap = document.createElement('div'); libWrap.className = 'stat-card'; libWrap.style.marginTop = '16px';
    libWrap.innerHTML = '<h4 style="margin:0 0 10px">Libraries (disk scan)</h4>';
    const libList = document.createElement('div'); libList.style.cssText = 'display:flex;flex-direction:column;gap:8px;margin-bottom:12px';
    const libForm = document.createElement('div'); libForm.style.cssText = 'display:flex;gap:8px;flex-wrap:wrap';
    const lname = document.createElement('input'); lname.placeholder = 'Library name';
    const lpaths = document.createElement('input'); lpaths.placeholder = 'Import paths (comma separated)'; lpaths.style.flex = '1';
    const lcreate = document.createElement('button'); lcreate.className = 'btn-primary'; lcreate.textContent = '+ Add library';
    lcreate.onclick = async () => {
      const r = await postJSON('/api/libraries', { name: lname.value || 'Library', importPaths: lpaths.value, type: 'EXTERNAL' });
      if (r.status === 201) { toast('Library created'); renderLibs(); }
      else toast('Create failed', false);
    };
    libForm.append(lname, lpaths, lcreate);
    libWrap.append(libList, libForm);
    view.appendChild(libWrap);

    async function renderLibs() {
      const r = await getJSON('/api/libraries');
      const libs = r.body || [];
      libList.innerHTML = '';
      for (const l of libs) {
        const row = document.createElement('div'); row.style.cssText = 'display:flex;gap:8px;align-items:center';
        const info = document.createElement('div'); info.style.flex = '1';
        info.innerHTML = '<b>' + escapeHtml(l.name) + '</b> <span class="muted">· ' + (l.status || '') + '</span><br><span class="muted" style="font-size:12px">' + escapeHtml(l.importPaths || 'no import paths') + '</span>';
        const scan = document.createElement('button'); scan.className = 'btn-ghost'; scan.textContent = 'Scan';
        scan.onclick = async () => {
          scan.disabled = true; scan.textContent = 'Scanning…';
          const sr = await postJSON('/api/libraries/' + l.id + '/scan', {});
          scan.disabled = false; scan.textContent = 'Scan';
          if (sr.status === 200) toast('Imported ' + (sr.body.imported || 0) + ', skipped ' + (sr.body.skipped || 0));
          else toast('Scan failed', false);
          renderLibs();
        };
        row.append(info, scan);
        libList.appendChild(row);
      }
    }
    renderLibs();
  }

  // ----------------------------------------------------------- tile render
  function renderTiles(container, assets, listRef, opts) {
    opts = opts || {};
    const grid = (container.classList && container.classList.contains('grid')) ? container : (() => {
      const g = document.createElement('div'); g.className = 'grid'; container.appendChild(g); return g;
    })();
    for (const a of assets) {
      const index = listRef.indexOf(a);
      grid.appendChild(makeTile(a, index, opts));
    }
  }

  function makeTile(asset, index, opts) {
    opts = opts || {};
    const tile = document.createElement('div');
    tile.className = 'tile';

    const img = document.createElement('img');
    img.dataset.id = asset.id;
    img.alt = asset.originalFileName || asset.id;
    tile.appendChild(img);

    if (asset.hasThumbnail) {
      if (thumbObserver) thumbObserver.observe(img);
      else (async () => { img.src = await thumbURL(asset.id); })();
    }

    if (asset.type === 'VIDEO') {
      const p = document.createElement('div'); p.className = 'play'; p.textContent = '▶';
      tile.appendChild(p);
    }
    if (asset.isFavorite) {
      const f = document.createElement('div'); f.className = 'fav'; f.textContent = '★';
      tile.appendChild(f);
    }

    // selection checkbox
    const sel = document.createElement('div');
    sel.className = 'sel' + (App.selected.has(asset.id) ? ' on' : '');
    sel.innerHTML = '<span>' + (App.selected.has(asset.id) ? '✓' : '') + '</span>';
    sel.onclick = (e) => { e.stopPropagation(); toggleSelect(asset.id); sel.className = 'sel' + (App.selected.has(asset.id) ? ' on' : ''); sel.innerHTML = '<span>' + (App.selected.has(asset.id) ? '✓' : '') + '</span>'; };
    tile.appendChild(sel);

    tile.onclick = () => openViewer(App.viewerList, index);
    return tile;
  }

  // ------------------------------------------------------------ selection
  function toggleSelect(id) {
    if (App.selected.has(id)) App.selected.delete(id); else App.selected.add(id);
    renderBulkBar();
  }
  function renderBulkBar() {
    let bar = $('bulk-bar');
    if (App.selected.size === 0) { if (bar) bar.remove(); return; }
    if (!bar) {
      bar = document.createElement('div'); bar.id = 'bulk-bar'; bar.className = 'toolbar';
      bar.style.position = 'sticky'; bar.style.top = '0'; bar.style.zIndex = '40';
      bar.style.background = 'var(--bg-elev)'; bar.style.border = '1px solid var(--border)';
      bar.style.borderRadius = '10px'; bar.style.padding = '10px 12px';
      $('view').prepend(bar);
    }
    bar.innerHTML = '';
    const info = document.createElement('span'); info.textContent = App.selected.size + ' selected';
    const sp = document.createElement('span'); sp.className = 'spacer';
    const fav = mkBtn('★ Favorite', () => bulkUpdate({ isFavorite: true }));
    const arch = mkBtn('▥ Archive', () => bulkUpdate({ isArchived: true }));
    const del = mkBtn('🗑 Trash', () => bulkUpdate({ isTrash: true }));
    const toAlbum = mkBtn('+ To album', async () => {
      const albumId = await chooseAlbum();
      if (!albumId) return;
      const res = await postJSON('/api/albums/' + albumId + '/assets', { ids: [...App.selected] });
      if (res.status === 200) { toast('Added to album'); App.selected.clear(); renderBulkBar(); }
      else toast('Failed', false);
    });
    const clear = mkBtn('Clear', () => { App.selected.clear(); renderBulkBar(); }, true);
    bar.append(info, sp, fav, arch, toAlbum, del, clear);
  }
  function mkBtn(label, fn, ghost) {
    const b = document.createElement('button');
    b.className = ghost ? 'btn-ghost' : 'btn-primary';
    b.textContent = label; b.onclick = fn; return b;
  }
  async function bulkUpdate(patch) {
    const ids = [...App.selected];
    await putJSON('/api/assets', Object.assign({ ids }, patch));
    toast('Updated ' + ids.length);
    App.selected.clear(); renderBulkBar();
    route();
  }

  // --------------------------------------------------------------- viewer
  function openViewer(list, index) {
    if (!list || index < 0 || index >= list.length) return;
    App.viewerList = list;
    App.viewerIndex = index;
    $('viewer').hidden = false;
    document.body.style.overflow = 'hidden';
    renderViewer();
  }
  function closeViewer() {
    $('viewer').hidden = true;
    document.body.style.overflow = '';
    $('viewer-stage').innerHTML = '';
  }
  function viewerStep(d) {
    const n = App.viewerList.length;
    if (n === 0) return;
    App.viewerIndex = (App.viewerIndex + d + n) % n;
    renderViewer();
  }
  function renderViewer() {
    const a = App.viewerList[App.viewerIndex];
    if (!a) return;
    const stage = $('viewer-stage');
    stage.innerHTML = '';
    if (a.type === 'VIDEO') {
      const v = document.createElement('video');
      v.controls = true; v.autoplay = true; v.muted = true; v.playsInline = true;
      const src = document.createElement('source');
      src.src = encURL(a.id); src.type = 'video/mp4';
      v.appendChild(src);
      v.addEventListener('error', () => {
        // fall back to original file if transcoded stream fails
        if (v.querySelector('source') && v.currentSrc.indexOf(origURL(a.id)) === -1) {
          const s2 = document.createElement('source'); s2.src = origURL(a.id); s2.type = 'video/mp4';
          v.appendChild(s2); v.load();
        }
      }, true);
      stage.appendChild(v);
    } else {
      // Image asset: show the still. For Live Photos, also auto-play the
      // looping motion video inline (overlaid on the still). The still stays
      // as a fallback if the motion stream fails to load.
      const wrap = document.createElement('div');
      wrap.className = 'live-wrap';
      const img = document.createElement('img');
      img.src = previewURL(a.id);
      wrap.appendChild(img);
      stage.appendChild(wrap);

      if (a.livePhotoVideoId) {
        const v = document.createElement('video');
        v.className = 'live-video';
        v.autoplay = true; v.loop = true; v.muted = true; v.playsInline = true;
        const s = document.createElement('source');
        s.src = '/api/assets/' + a.id + '/live-photo';
        s.type = 'video/mp4';
        v.appendChild(s);
        // if the motion stream is unavailable, fall back to the still image
        v.addEventListener('error', () => { v.style.display = 'none'; }, true);
        wrap.appendChild(v);
        // kick off playback (muted autoplay can be blocked until user gesture)
        const tryPlay = () => { const p = v.play(); if (p && p.catch) p.catch(() => {}); };
        tryPlay();
        v.addEventListener('canplay', tryPlay, { once: true });

        const live = document.createElement('button');
        live.className = 'live-badge';
        live.textContent = '● LIVE';
        live.title = 'Play Live Photo motion full-screen';
        live.onclick = () => showLivePhoto(a);
        stage.appendChild(live);
      }
    }
    renderPanel(a);
  }
  // showLivePhoto replaces the still image in the viewer with the motion
  // (video) component streamed by the backend's /api/assets/:id/live-photo.
  function showLivePhoto(a) {
    const stage = $('viewer-stage');
    stage.innerHTML = '';
    const v = document.createElement('video');
    v.controls = true; v.autoplay = true; v.loop = true; v.muted = true; v.playsInline = true;
    const s = document.createElement('source');
    s.src = '/api/assets/' + a.id + '/live-photo';
    s.type = 'video/mp4';
    v.appendChild(s);
    v.addEventListener('error', () => {
      if (v.currentSrc.indexOf('/live-photo') !== -1) toast('Live Photo motion unavailable', false);
    }, true);
    stage.appendChild(v);
  }
  function renderPanel(a) {
    const ex = a.exif || {};
    const p = $('viewer-panel');
    p.innerHTML = '';
    const title = document.createElement('div');
    title.className = 'vp-title'; title.textContent = a.originalFileName || a.id;
    p.appendChild(title);

    const rows = [];
    rows.push(['Type', a.type]);
    if (a.localDateTime) rows.push(['Date', fmtDate(a.localDateTime)]);
    const cam = [ex.make, ex.model].filter(Boolean).join(' ').trim();
    if (cam) rows.push(['Camera', cam]);
    if (ex.city || ex.country) rows.push(['Location', [ex.city, ex.country].filter(Boolean).join(', ')]);
    if (a.type === 'VIDEO' && a.duration) rows.push(['Duration', a.duration]);
    for (const [k, v] of rows) {
      const row = document.createElement('div'); row.className = 'vp-row';
      row.innerHTML = '<span class="k"></span><span class="v"></span>';
      row.querySelector('.k').textContent = k; row.querySelector('.v').textContent = v;
      p.appendChild(row);
    }

    const actions = document.createElement('div'); actions.className = 'vp-actions';
    const favBtn = mkBtn(a.isFavorite ? '★ Favorited' : '☆ Favorite', async () => {
      await putJSON('/api/assets/' + a.id, { isFavorite: !a.isFavorite });
      a.isFavorite = !a.isFavorite; renderPanel(a);
    });
    const archBtn = mkBtn(a.isArchived ? '▥ Archived' : '▥ Archive', async () => {
      await putJSON('/api/assets/' + a.id, { isArchived: !a.isArchived });
      a.isArchived = !a.isArchived; renderPanel(a);
    });
    const dl = mkBtn('↓ Download', () => { window.open(origURL(a.id), '_blank'); }, true);
    const addAlbum = mkBtn('+ Album', async () => {
      const albumId = await chooseAlbum();
      if (!albumId) return;
      const res = await postJSON('/api/albums/' + albumId + '/assets', { ids: [a.id] });
      if (res.status === 200) toast('Added to album');
    }, true);
    const share = mkBtn('🔗 Share', async () => {
      const r = await postJSON('/api/shared-links', { type: 'INDIVIDUAL', assetId: a.id });
      if (r.status === 201 && r.body && r.body.key) {
        const url = location.origin + '/share/' + r.body.key;
        try { await navigator.clipboard.writeText(url); } catch (_) {}
        toast('Share link copied to clipboard');
        window.prompt('Share link (copied):', url);
      } else toast('Could not create share', false);
    }, true);
    const del = mkBtn('🗑 Delete', async () => {
      if (!confirm('Move this asset to trash?')) return;
      await delJSON('/api/assets', { ids: [a.id] });
      toast('Moved to trash'); closeViewer(); route();
    }, true);
    if (a.livePhotoVideoId) {
      const liveBtn = mkBtn('▶ Live Photo', () => showLivePhoto(a), true);
      actions.append(liveBtn);
    }
    actions.append(favBtn, archBtn, addAlbum, share, dl, del);
    p.appendChild(actions);
  }

  // ----------------------------------------------------------- album picker
  function chooseAlbum() {
    return new Promise(async (resolve) => {
      const r = await getJSON('/api/albums');
      const albums = r.body || [];
      const overlay = document.createElement('div');
      overlay.style.cssText = 'position:fixed;inset:0;z-index:200;background:rgba(0,0,0,.6);display:flex;align-items:center;justify-content:center';
      const box = document.createElement('div');
      box.style.cssText = 'background:var(--bg-elev);border:1px solid var(--border);border-radius:14px;padding:18px;width:340px;max-width:92vw';
      box.innerHTML = '<h3 style="margin:0 0 12px">Add to album</h3>';
      const list = document.createElement('div');
      list.style.cssText = 'max-height:240px;overflow:auto;display:flex;flex-direction:column;gap:6px';
      if (albums.length === 0) list.innerHTML = '<div class="muted">No albums yet</div>';
      albums.forEach((al) => {
        const b = document.createElement('button'); b.className = 'btn-ghost';
        b.style.textAlign = 'left'; b.textContent = al.albumName + '  (' + (al.assetCount || 0) + ')';
        b.onclick = () => { overlay.remove(); resolve(al.id); };
        list.appendChild(b);
      });
      const newWrap = document.createElement('div'); newWrap.style.cssText = 'margin-top:12px;display:flex;gap:8px';
      const ni = document.createElement('input'); ni.placeholder = 'New album name…';
      const nc = document.createElement('button'); nc.className = 'btn-primary'; nc.textContent = 'Create';
      nc.onclick = async () => {
        const name = ni.value.trim(); if (!name) return;
        const cr = await postJSON('/api/albums', { albumName: name });
        overlay.remove(); resolve(cr.body && cr.body.id);
      };
      newWrap.append(ni, nc);
      const cancel = document.createElement('button'); cancel.className = 'btn-ghost'; cancel.textContent = 'Cancel';
      cancel.style.marginTop = '12px'; cancel.onclick = () => { overlay.remove(); resolve(null); };
      box.append(list, newWrap, cancel);
      overlay.appendChild(box);
      overlay.onclick = (e) => { if (e.target === overlay) { overlay.remove(); resolve(null); } };
      document.body.appendChild(overlay);
    });
  }

  // asset picker (returns chosen ids) — used for "set cover"
  function pickAsset(cb) {
    getJSON('/api/assets?take=100').then((r) => {
      const assets = (r.body && r.body.assets) || [];
      pickAssets(assets, 'Choose asset').then((ids) => { if (ids && ids[0]) cb(ids[0]); });
    });
  }
  function pickAssets(assets, title) {
    return new Promise((resolve) => {
      const overlay = document.createElement('div');
      overlay.style.cssText = 'position:fixed;inset:0;z-index:200;background:rgba(0,0,0,.6);display:flex;align-items:center;justify-content:center';
      const box = document.createElement('div');
      box.style.cssText = 'background:var(--bg-elev);border:1px solid var(--border);border-radius:14px;padding:18px;width:min(720px,94vw);max-height:84vh;display:flex;flex-direction:column';
      box.innerHTML = '<h3 style="margin:0 0 12px">' + (title || 'Select') + '</h3>';
      const grid = document.createElement('div'); grid.className = 'grid';
      grid.style.cssText = 'overflow:auto;flex:1;align-content:start';
      const chosen = new Set();
      assets.forEach((a) => {
        const t = makeTile(a, 0, {});
        t.onclick = (e) => {
          e.stopPropagation();
          if (chosen.has(a.id)) { chosen.delete(a.id); t.classList.remove('selected'); }
          else { chosen.add(a.id); t.classList.add('selected'); }
        };
        grid.appendChild(t);
      });
      const bar = document.createElement('div'); bar.className = 'toolbar'; bar.style.marginTop = '12px';
      const sp = document.createElement('span'); sp.className = 'spacer';
      const ok = document.createElement('button'); ok.className = 'btn-primary'; ok.textContent = 'Choose';
      ok.onclick = () => { overlay.remove(); resolve([...chosen]); };
      const cancel = document.createElement('button'); cancel.className = 'btn-ghost'; cancel.textContent = 'Cancel';
      cancel.onclick = () => { overlay.remove(); resolve(null); };
      bar.append(sp, ok, cancel);
      box.append(grid, bar);
      overlay.appendChild(box);
      document.body.appendChild(overlay);
    });
  }

  // --------------------------------------------------------------- upload
  function initUpload() {
    const fileInput = document.createElement('input');
    fileInput.type = 'file'; fileInput.id = 'file-input';
    fileInput.multiple = true; fileInput.accept = 'image/*,video/*';
    document.body.appendChild(fileInput);

    $('upload-btn').onclick = () => fileInput.click();
    fileInput.onchange = () => { if (fileInput.files.length) uploadFiles(fileInput.files); fileInput.value = ''; };

    const dz = $('dropzone');
    let depth = 0;
    window.addEventListener('dragenter', (e) => { if (App.token) { e.preventDefault(); depth++; dz.hidden = false; } });
    window.addEventListener('dragover', (e) => { if (App.token) e.preventDefault(); });
    window.addEventListener('dragleave', (e) => { depth--; if (depth <= 0) { depth = 0; dz.hidden = true; } });
    window.addEventListener('drop', (e) => {
      e.preventDefault(); depth = 0; dz.hidden = true;
      if (e.dataTransfer && e.dataTransfer.files.length) uploadFiles(e.dataTransfer.files);
    });
  }
  async function uploadFiles(fileList) {
    const files = Array.from(fileList);
    let done = 0;
    toast('Uploading ' + files.length + ' file(s)…');
    for (const f of files) {
      const type = f.type.startsWith('video') ? 'VIDEO' : 'IMAGE';
      const meta = {
        deviceAssetId: f.name + '-' + f.size + '-' + f.lastModified,
        deviceId: 'web',
        fileCreatedAt: new Date(f.lastModified || Date.now()).toISOString(),
        fileModifiedAt: new Date(f.lastModified || Date.now()).toISOString(),
        localDateTime: new Date(f.lastModified || Date.now()).toISOString(),
        fileExtension: '.' + (f.name.split('.').pop() || 'jpg'),
        type, isFavorite: false, isArchived: false,
      };
      const fd = new FormData();
      fd.append('asset', JSON.stringify(meta));
      fd.append('assetData', f);
      try {
        const r = await fetch('/api/assets', { method: 'POST', headers: authHeaders(false), body: fd });
        if (r.status !== 201) toast('Failed: ' + f.name, false);
      } catch (_) { toast('Upload error: ' + f.name, false); }
      done++;
    }
    toast('Uploaded ' + done + '/' + files.length);
    route();
  }

// --------------------------------------------------------------- helpers
function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function fmtDate(s) {
    const d = new Date(s);
    if (isNaN(d)) return s;
    return d.toLocaleString();
  }
  function formatBucket(s) {
    const d = new Date(s);
    if (isNaN(d)) return s;
    return d.toLocaleDateString(undefined, { year: 'numeric', month: 'long' });
  }

  // --------------------------------------------------------------- wiring
  // --------------------------------------------------- realtime sync (ws)
  let syncWS = null;
  let syncRetry = 0;
  let syncRefreshTimer = null;

  function connectSync() {
    if (!App.token) return;
    if (syncWS && syncWS.readyState <= 1) return; // already connecting/open
    const proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
    let ws;
    try {
      ws = new WebSocket(proto + location.host + '/api/events');
    } catch (e) {
      scheduleSyncReconnect();
      return;
    }
    syncWS = ws;
    ws.onopen = () => { syncRetry = 0; };
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (_) { return; }
      if (!msg || !msg.type || msg.type === 'init') return;
      // A realtime event arrived (asset/album change): debounce-refresh the
      // current view so another client's changes show up without a manual reload.
      scheduleSyncRefresh();
    };
    ws.onclose = () => { syncWS = null; scheduleSyncReconnect(); };
    ws.onerror = () => { try { syncWS.close(); } catch (_) {} };
  }
  function scheduleSyncReconnect() {
    syncRetry++;
    const delay = Math.min(30000, 1000 * Math.pow(2, Math.min(syncRetry, 5)));
    setTimeout(connectSync, delay);
  }
  function scheduleSyncRefresh() {
    if (syncRefreshTimer) clearTimeout(syncRefreshTimer);
    syncRefreshTimer = setTimeout(() => {
      syncRefreshTimer = null;
      if (App.token) route(); // re-render current view from the API
    }, 600);
  }

  function init() {
    // login
    $('login-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const r = await postJSON('/api/auth/login', { email: $('email').value, password: $('pass').value });
      if (r.status === 200 && r.body && r.body.accessToken) {
        App.token = r.body.accessToken;
        localStorage.setItem('immich_token', App.token);
        const me = await getJSON('/api/users/me');
        App.user = me.body;
        showApp();
        connectSync();
      } else {
        $('login-msg').className = 'msg err';
        $('login-msg').textContent = 'Sign in failed';
      }
    });
    $('logout-btn').onclick = () => {
      localStorage.removeItem('immich_token'); App.token = null; App.user = null; showLogin();
    };

    // viewer controls
    $('viewer-close').onclick = closeViewer;
    $('viewer-prev').onclick = () => viewerStep(-1);
    $('viewer-next').onclick = () => viewerStep(1);
    $('viewer').addEventListener('click', (e) => { if (e.target === $('viewer')) closeViewer(); });
    document.addEventListener('keydown', (e) => {
      if ($('viewer').hidden) return;
      if (e.key === 'Escape') closeViewer();
      else if (e.key === 'ArrowLeft') viewerStep(-1);
      else if (e.key === 'ArrowRight') viewerStep(1);
    });

    window.addEventListener('hashchange', route);
    initUpload();
    bootstrap();
    connectSync();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
