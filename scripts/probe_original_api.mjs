// Live API probe for the ORIGINAL Immich server (v3.1.0).
// Runs INSIDE the immich-server container; talks to 127.0.0.1:2283.
// Produces: /tmp/probe-out/probe-results.json + dto-samples.json + coverage.json
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';

const BASE = process.env.PROBE_BASE || 'http://127.0.0.1:2283/api';
const OUT = '/tmp/probe-out';
mkdirSync(OUT, { recursive: true });
const spec = JSON.parse(readFileSync('/tmp/spec.json', 'utf8'));

const results = [];   // every real request
const samples = {};   // full bodies of key DTOs
const ctx = {};       // real ids discovered while probing
let TOKEN = null, TOKEN2 = null, PASSWORD = 'password';

const tryJson = t => { try { return JSON.parse(t); } catch { return t; } };
const sleep = ms => new Promise(r => setTimeout(r, ms));
const U = () => crypto.randomUUID();

async function call(group, method, path, opts = {}) {
  const url = BASE + path + (opts.query ? '?' + new URLSearchParams(Object.fromEntries(Object.entries(opts.query).filter(([, v]) => v !== undefined && v !== null).map(([k, v]) => [k, String(v)]))) : '');
  const headers = { ...(opts.headers || {}) };
  if (opts.token) headers.Authorization = 'Bearer ' + opts.token;
  let body;
  if (opts.form) body = opts.form;
  else if (opts.body !== undefined) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(opts.body); }
  const rec = { group, method, path, status: 0, body: null, err: null, note: opts.note || '' };
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const r = await fetch(url, { method, headers, body });
      rec.status = r.status;
      // strip control chars (binary endpoints would otherwise inject NULs)
      const t = (await r.text()).replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g, '');
      const b = t ? tryJson(t) : null;
      rec.body = typeof b === 'string' ? b.slice(0, 300) : b;
      if (r.headers.getSetCookie) { const sc = r.headers.getSetCookie(); if (sc.length) rec.setCookie = sc; }
      if (opts.sample && rec.body !== null) samples[opts.sample] = rec.body;
      rec.err = null;
      break;
    } catch (e) {
      rec.err = String(e.message || e);
      rec.status = 0;
      if (attempt < 2) await sleep(4000);
    }
  }
  results.push(rec);
  return rec;
}

function png(name) {
  const bytes = readFileSync('/tmp/probe-assets/' + name);
  return new Blob([bytes], { type: 'image/png' });
}
function uploadForm(name, id, date, mime) {
  const fd = new FormData();
  fd.append('assetData', png(name), name);
  fd.append('deviceAssetId', id);
  fd.append('deviceId', 'probe-device');
  fd.append('fileCreatedAt', date);
  fd.append('fileModifiedAt', date);
  fd.append('duration', '0');
  return fd;
}
async function uploadOne(group, name, id, date) {
  const r = await call(group, 'POST', '/assets', { token: TOKEN, form: uploadForm(name, id, date), sample: 'asset-upload', note: 'multipart upload' });
  const b = r.body;
  const id2 = Array.isArray(b) ? b[0]?.id : b?.id;
  if (id2) ctx['asset:' + name] = id2;
  return r;
}

// ---------------------------------------------------------------- scenario
async function main() {
  // ---- 00 server, public
  await call('server', 'GET', '/server/ping', { sample: 'server-ping' });
  await call('server', 'GET', '/server/version', { sample: 'server-version' });
  await call('server', 'GET', '/server/features', { sample: 'server-features' });
  await call('server', 'GET', '/server/config', { sample: 'server-config' });
  await call('server', 'GET', '/server/about', { sample: 'server-about' });
  await call('server', 'GET', '/server/media-types', { sample: 'server-media-types' });
  await call('server', 'GET', '/server/theme', { sample: 'server-theme' });
  await call('server', 'GET', '/server/statistics', { sample: 'server-statistics', note: 'public?' });

  // ---- 01 auth
  await call('auth', 'POST', '/auth/admin-sign-up', { body: { email: 'admin@immich.app', password: 'password', name: 'Administrator' }, note: 'already created -> 400 expected', sample: 'auth-admin-signup-dup' });
  let r = await call('auth', 'POST', '/auth/login', { body: { email: 'admin@immich.app', password: 'password' }, sample: 'auth-login' });
  TOKEN = r.body?.accessToken;
  ctx.adminId = r.body?.userId;
  await call('auth', 'POST', '/auth/validateToken', { token: TOKEN, body: {}, sample: 'auth-validate' });
  await call('auth', 'POST', '/auth/admin-sign-up', { token: TOKEN, body: { email: 'x@y.z', password: 'pw12345678', name: 'X' }, note: 'signup with existing admin' });

  // ---- 02 users/me
  await call('users', 'GET', '/users/me', { token: TOKEN, sample: 'users-me' });
  await call('users', 'GET', '/users/me/preferences', { token: TOKEN, sample: 'users-me-preferences' });
  await call('users', 'PUT', '/users/me/preferences', { token: TOKEN, body: { tags: { enabled: true } }, sample: 'users-me-preferences-put' });
  await call('users', 'GET', '/users/me/statistics', { token: TOKEN, sample: 'users-me-statistics' });
  await call('users', 'GET', '/users', { token: TOKEN, query: { withHidden: 'false' }, sample: 'users-all' });

  // ---- 03 upload assets
  await uploadOne('assets', 'a1.png', 'probe-a1', '2021-06-15T10:00:00.000Z');
  await uploadOne('assets', 'a2.png', 'probe-a2', '2022-08-20T12:00:00.000Z');
  await uploadOne('assets', 'a3.png', 'probe-a3', '2023-01-05T08:30:00.000Z');
  // duplicate upload of a1 (same deviceAssetId+deviceId) -> record duplicate handling
  await uploadOne('assets', 'a1.png', 'probe-a1', '2021-06-15T10:00:00.000Z');

  ctx.asset1 = ctx['asset:a1.png']; ctx.asset2 = ctx['asset:a2.png']; ctx.asset3 = ctx['asset:a3.png'];

  // ---- 04 assets read/update
  await call('assets', 'GET', `/assets/${ctx.asset1}`, { token: TOKEN, sample: 'asset-get' });
  await call('assets', 'GET', `/assets/${ctx.asset1}/original`, { token: TOKEN, note: 'binary' });
  await call('assets', 'GET', `/assets/${ctx.asset1}/thumbnail?size=thumbnail`, { token: TOKEN, note: 'binary' });
  await call('assets', 'GET', `/assets/${ctx.asset1}/preview`, { token: TOKEN, note: 'binary' });
  await call('assets', 'GET', `/assets/${ctx.asset1}/video/playback`, { token: TOKEN, note: 'non-video asset' });
  await call('assets', 'PUT', '/assets', { token: TOKEN, body: { ids: [ctx.asset2], isFavorite: true }, sample: 'assets-bulk-update' });
  await call('assets', 'GET', `/assets/${ctx.asset2}`, { token: TOKEN, note: 'check isFavorite persisted' });
  await call('assets', 'GET', '/assets', { token: TOKEN, query: { withArchived: 'true' }, sample: 'assets-get-all' });
  await call('assets', 'GET', '/assets/device/probe-device', { token: TOKEN, sample: 'assets-by-device' });
  await call('assets', 'GET', '/assets/random?count=2', { token: TOKEN, sample: 'assets-random' });
  await call('assets', 'GET', '/assets/statistics', { token: TOKEN, sample: 'assets-statistics' });
  await call('assets', 'GET', `/assets/${ctx.asset1}/statistics`, { token: TOKEN, note: 'per-asset statistics?' });

  // ---- 05 timeline
  const tb = await call('timeline', 'GET', '/timeline/buckets', { token: TOKEN, query: { withPartners: 'false', withStacked: 'true' }, sample: 'timeline-buckets' });
  let bucket = tb.body?.[0]?.timeBucket;
  if (bucket) await call('timeline', 'GET', '/timeline/bucket', { token: TOKEN, query: { timeBucket: bucket }, sample: 'timeline-bucket' });
  await call('timeline', 'GET', '/timeline/bucket', { token: TOKEN, query: { timeBucket: '2021-06-01T00:00:00.000Z' }, sample: 'timeline-bucket-explicit' });

  // ---- 06 albums
  const alb = await call('albums', 'POST', '/albums', { token: TOKEN, body: { albumName: 'Probe Album', description: 'created by probe' }, sample: 'album-create' });
  ctx.albumId = alb.body?.id;
  await call('albums', 'GET', '/albums', { token: TOKEN, sample: 'albums-list' });
  await call('albums', 'GET', `/albums/${ctx.albumId}`, { token: TOKEN, sample: 'album-get' });
  await call('albums', 'PATCH', `/albums/${ctx.albumId}`, { token: TOKEN, body: { albumName: 'Probe Album renamed' }, sample: 'album-patch' });
  await call('albums', 'PUT', `/albums/${ctx.albumId}/assets`, { token: TOKEN, body: { ids: [ctx.asset1, ctx.asset2] }, sample: 'album-add-assets' });
  await call('albums', 'GET', `/albums/${ctx.albumId}/assets`, { token: TOKEN, sample: 'album-assets' });
  await call('albums', 'PUT', '/albums/assets', { token: TOKEN, body: { albumIds: [ctx.albumId], assetIds: [ctx.asset3] }, sample: 'albums-bulk-add' });
  await call('albums', 'GET', '/albums/statistics', { token: TOKEN, sample: 'albums-statistics' });
  await call('albums', 'GET', `/albums/${ctx.albumId}/statistics`, { token: TOKEN, sample: 'album-statistics' });

  // ---- 07 activities (comments on album)
  await call('activities', 'GET', '/activities', { token: TOKEN, query: { albumId: ctx.albumId }, sample: 'activities-list' });
  const act = await call('activities', 'POST', '/activities', { token: TOKEN, body: { albumId: ctx.albumId, comment: 'probe comment' }, sample: 'activity-create' });
  ctx.activityId = act.body?.id;
  await call('activities', 'GET', '/activities/statistics', { token: TOKEN, query: { albumId: ctx.albumId }, sample: 'activities-statistics' });
  if (ctx.activityId) await call('activities', 'DELETE', `/activities/${ctx.activityId}`, { token: TOKEN });

  // ---- 08 tags
  const tag = await call('tags', 'POST', '/tags', { token: TOKEN, body: { name: 'Probe/Tag', color: '#ff0000' }, sample: 'tag-create' });
  ctx.tagId = tag.body?.id;
  await call('tags', 'GET', '/tags', { token: TOKEN, sample: 'tags-list' });
  if (ctx.tagId) {
    await call('tags', 'GET', `/tags/${ctx.tagId}`, { token: TOKEN, sample: 'tag-get' });
    await call('tags', 'PUT', `/tags/${ctx.tagId}`, { token: TOKEN, body: { name: 'Probe/Renamed', color: 'blue' }, sample: 'tag-update' });
    await call('tags', 'PUT', '/tags/assets', { token: TOKEN, body: { tagIds: [ctx.tagId], assetIds: [ctx.asset1] }, sample: 'tag-bulk-assets' });
    await call('tags', 'POST', `/tags/${ctx.tagId}/assets`, { token: TOKEN, body: { assetIds: [ctx.asset2] }, sample: 'tag-add-assets' });
  }

  // ---- 09 memories
  const mem = await call('memories', 'POST', '/memories', { token: TOKEN, body: { type: 'on_this_day', data: { year: 2021 }, memoryAt: '2021-06-15T09:00:00.000Z', assetIds: [ctx.asset1] }, sample: 'memory-create' });
  ctx.memoryId = mem.body?.id;
  await call('memories', 'GET', '/memories', { token: TOKEN, sample: 'memories-list' });
  if (ctx.memoryId) {
    await call('memories', 'GET', `/memories/${ctx.memoryId}`, { token: TOKEN, sample: 'memory-get' });
    await call('memories', 'PUT', `/memories/${ctx.memoryId}`, { token: TOKEN, body: { isSaved: true }, sample: 'memory-update' });
    await call('memories', 'PUT', `/memories/${ctx.memoryId}/assets`, { token: TOKEN, body: { ids: [ctx.asset2] }, sample: 'memory-add-assets' });
  }

  // ---- 10 stacks
  const stk = await call('stacks', 'POST', '/stacks', { token: TOKEN, body: { assetIds: [ctx.asset1, ctx.asset3] }, sample: 'stack-create' });
  ctx.stackId = stk.body?.id;
  await call('stacks', 'GET', '/stacks', { token: TOKEN, sample: 'stacks-list' });
  if (ctx.stackId) {
    await call('stacks', 'GET', `/stacks/${ctx.stackId}`, { token: TOKEN, sample: 'stack-get' });
    await call('stacks', 'PUT', `/stacks/${ctx.stackId}`, { token: TOKEN, body: { primaryAssetId: ctx.asset3 }, sample: 'stack-update' });
  }

  // ---- 11 partners (second user)
  const u2 = await call('admin', 'POST', '/admin/users', { token: TOKEN, body: { email: 'partner@immich.app', name: 'Partner', password: 'partner12345' }, sample: 'admin-create-user' });
  ctx.userId2 = u2.body?.id;
  r = await call('auth', 'POST', '/auth/login', { body: { email: 'partner@immich.app', password: 'partner12345' }, sample: 'auth-login-partner' });
  TOKEN2 = r.body?.accessToken;
  if (ctx.userId2 && TOKEN) await call('partners', 'PUT', `/partners/${ctx.userId2}`, { token: TOKEN, sample: 'partner-create' });
  await call('partners', 'GET', '/partners', { token: TOKEN, query: { direction: 'shared-by-me' }, sample: 'partners-list' });
  await call('partners', 'GET', '/partners', { token: TOKEN2, query: { direction: 'shared-with-me' }, sample: 'partners-list-partner' });

  // ---- 12 shared links
  const sl = await call('shared-links', 'POST', '/shared-links', { token: TOKEN, body: { type: 'ALBUM', albumId: ctx.albumId, allowDownload: true, allowUpload: false, description: 'probe' }, sample: 'shared-link-create' });
  ctx.sharedLinkId = sl.body?.id; ctx.sharedLinkKey = sl.body?.key;
  await call('shared-links', 'GET', '/shared-links', { token: TOKEN, sample: 'shared-links-list' });
  if (ctx.sharedLinkId) {
    await call('shared-links', 'GET', `/shared-links/me`, { token: TOKEN, note: 'list mine' });
    await call('shared-links', 'GET', `/shared-links/${ctx.sharedLinkId}`, { token: TOKEN, sample: 'shared-link-get' });
    await call('shared-links', 'PATCH', `/shared-links/${ctx.sharedLinkId}`, { token: TOKEN, body: { description: 'renamed' }, sample: 'shared-link-patch' });
  }
  if (ctx.sharedLinkKey) await call('shared-links', 'GET', `/share/${ctx.sharedLinkKey}`, { note: 'public, no auth', sample: 'share-public' });

  // ---- 13 trash
  await call('trash', 'PUT', '/assets', { token: TOKEN, body: { ids: [ctx.asset3], trash: true }, sample: 'assets-trash', note: 'move asset3 to trash' });
  await call('trash', 'GET', '/trash', { token: TOKEN, sample: 'trash-list' });
  await call('trash', 'PUT', '/trash/restore/assets', { token: TOKEN, body: { ids: [ctx.asset3] }, sample: 'trash-restore-assets' });

  // ---- 14 search
  await call('search', 'GET', '/search/metadata', { token: TOKEN, query: { isFavorite: 'true' }, sample: 'search-metadata' });
  await call('search', 'POST', '/search/metadata', { token: TOKEN, body: { take: 10 }, sample: 'search-metadata-post' });
  await call('search', 'POST', '/search/smart', { token: TOKEN, body: { query: 'dog' }, note: 'ML disabled' });
  await call('search', 'GET', '/search/explore', { token: TOKEN, sample: 'search-explore' });
  await call('search', 'GET', '/search/suggestions', { token: TOKEN, query: { country: 'Japan' }, sample: 'search-suggestions' });
  await call('search', 'GET', '/search/cities', { token: TOKEN, sample: 'search-cities' });
  await call('search', 'POST', '/search/places', { token: TOKEN, body: { name: 'Tokyo' }, sample: 'search-places' });
  await call('search', 'GET', '/search/person', { token: TOKEN, note: 'deprecated?' });

  // ---- 15 map
  await call('map', 'GET', '/map/markers', { token: TOKEN, sample: 'map-markers' });
  await call('map', 'GET', '/map/reverse-geocode', { token: TOKEN, query: { lat: '35.6895', lon: '139.6917' }, sample: 'map-reverse-geocode' });

  // ---- 16 jobs
  await call('jobs', 'GET', '/jobs', { token: TOKEN, sample: 'jobs-all' });
  await call('jobs', 'PUT', '/jobs/thumbnailGeneration', { token: TOKEN, body: { command: 'start', force: false }, sample: 'jobs-start' });

  // ---- 17 system config / admin users
  await call('admin', 'GET', '/system-config', { token: TOKEN, sample: 'system-config' });
  await call('admin', 'GET', '/system-config/defaults', { token: TOKEN, sample: 'system-config-defaults' });
  await call('admin', 'GET', '/system-config/storage-template-options', { token: TOKEN });
  await call('admin', 'GET', '/admin/users', { token: TOKEN, sample: 'admin-users-list' });
  if (ctx.userId2) {
    await call('admin', 'GET', `/admin/users/${ctx.userId2}`, { token: TOKEN, sample: 'admin-user-get' });
    await call('admin', 'GET', `/admin/users/${ctx.userId2}/statistics`, { token: TOKEN, sample: 'admin-user-statistics' });
    await call('admin', 'GET', `/admin/users/${ctx.userId2}/preferences`, { token: TOKEN, sample: 'admin-user-preferences' });
    await call('admin', 'GET', `/admin/users/${ctx.userId2}/calendar-heatmap`, { token: TOKEN, sample: 'admin-user-calendar-heatmap' });
    await call('admin', 'GET', `/admin/users/${ctx.userId2}/sessions`, { token: TOKEN, sample: 'admin-user-sessions' });
    await call('admin', 'PUT', `/admin/users/${ctx.userId2}`, { token: TOKEN, body: { shouldChangePassword: false }, sample: 'admin-user-update' });
  }
  await call('admin', 'GET', '/admin/notifications', { token: TOKEN, sample: 'admin-notifications-list' });

  // ---- 18 sessions & api keys
  await call('sessions', 'GET', '/sessions', { token: TOKEN, sample: 'sessions-list' });
  const ak = await call('api-keys', 'POST', '/api-keys', { token: TOKEN, body: { name: 'probe-key', permissions: ['asset.read', 'asset.update'] }, sample: 'api-key-create' });
  ctx.apiKeyId = ak.body?.id;
  await call('api-keys', 'GET', '/api-keys', { token: TOKEN, sample: 'api-keys-list' });
  if (ctx.apiKeyId) await call('api-keys', 'GET', `/api-keys/${ctx.apiKeyId}`, { token: TOKEN, sample: 'api-key-get' });

  // ---- 19 libraries
  const lib = await call('libraries', 'POST', '/libraries', { token: TOKEN, body: { name: 'Probe Lib', importPaths: ['/data/import'], exclusionPatterns: [] }, sample: 'library-create' });
  ctx.libraryId = lib.body?.id;
  await call('libraries', 'GET', '/libraries', { token: TOKEN, sample: 'libraries-list' });
  if (ctx.libraryId) {
    await call('libraries', 'GET', `/libraries/${ctx.libraryId}`, { token: TOKEN, sample: 'library-get' });
    await call('libraries', 'PUT', `/libraries/${ctx.libraryId}`, { token: TOKEN, body: { name: 'Probe Lib 2' }, sample: 'library-update' });
    await call('libraries', 'POST', `/libraries/${ctx.libraryId}/validate`, { token: TOKEN, body: { importPaths: ['/data/import'], exclusionPatterns: [] }, sample: 'library-validate' });
    await call('libraries', 'GET', `/libraries/${ctx.libraryId}/statistics`, { token: TOKEN, sample: 'library-statistics' });
    await call('libraries', 'POST', `/libraries/${ctx.libraryId}/scan`, { token: TOKEN, body: {}, sample: 'library-scan' });
  }

  // ---- 20 notifications (user)
  await call('notifications', 'GET', '/notifications', { token: TOKEN, sample: 'notifications-list' });
  await call('notifications', 'PUT', '/notifications', { token: TOKEN, body: { ids: [], readAt: '2026-09-20T00:00:00.000Z' }, sample: 'notifications-update' });

  // ---- 21 people / faces (ML disabled)
  await call('people', 'GET', '/people', { token: TOKEN, query: { withHidden: 'false' }, sample: 'people-list' });
  await call('people', 'GET', '/people/statistics', { token: TOKEN, sample: 'people-statistics' });

  // ---- 22 duplicates
  await call('duplicates', 'GET', '/assets/duplicates', { token: TOKEN, sample: 'duplicates-list' });

  // ---- 23 oauth / license / pin / sync / download / misc
  await call('oauth', 'GET', '/oauth/config', { token: TOKEN, sample: 'oauth-config' });
  await call('oauth', 'GET', '/oauth/authorize', { query: { redirectUri: 'http://x' }, note: 'no oauth provider' });
  await call('license', 'GET', '/users/me/license', { token: TOKEN, sample: 'license-get' });
  await call('license', 'DELETE', '/users/me/license', { token: TOKEN });
  await call('auth', 'GET', '/auth/pin-code', { token: TOKEN, sample: 'pin-get' });
  await call('sync', 'POST', '/sync/stream', { token: TOKEN, body: [{ types: ['AssetV1'], ack: 'init' }], sample: 'sync-stream' });
  await call('sync', 'GET', '/sync/ack', { token: TOKEN, sample: 'sync-ack' });
  await call('download', 'POST', '/download/info', { token: TOKEN, body: { assetIds: [ctx.asset1] }, sample: 'download-info' });
  await call('misc', 'GET', '/users/me/media-favorite', { token: TOKEN, note: 'exists?' });

  // ---- 24 second-user perspective (partner sees what?)
  await call('auth2', 'GET', '/users/me', { token: TOKEN2, sample: 'users-me-partner' });

  // ---- 25 coverage of ALL spec operations not yet exercised
  const covered = new Set(results.filter(r => r.status !== 0).map(r => r.method + ' ' + r.path.split('?')[0]));
  const byPath = {};
  for (const p of Object.keys(spec.paths)) byPath[p.replace(/^\//, '')] = p;

  function matchPath(method, p) {
    // find result whose recorded path (after BASE) equals p with {x} filled
    return [...covered].some(c => {
      const sp = c.indexOf(' ');
      const m = c.slice(0, sp);
      if (m !== method) return false;
      const pa = c.slice(sp + 1).split('?')[0];
      const ps = p.split('/'), ca = pa.split('/');
      if (ps.length !== ca.length) return false;
      return ps.every((seg, i) => (seg.startsWith('{') && i > 0) || seg === ca[i]);
    });
  }

  for (const [p, ms] of Object.entries(spec.paths)) {
    for (const [m, op] of Object.entries(ms)) {
      if (!['get', 'post', 'put', 'patch', 'delete'].includes(m)) continue;
      const method = m.toUpperCase();
      if (matchPath(method, p)) continue;
      // not yet covered -> generic attempt with example values
      let real = p;
      const segs = [];
      for (const seg of p.split('/')) {
        if (seg.startsWith('{')) {
          const key = seg.slice(1, -1);
          const val = ctx[key] || ctx[Object.keys(ctx).find(k => k.toLowerCase().includes(key.toLowerCase())) || ''] || op.parameters?.find(q => q.name === key)?.schema?.example || op.parameters?.find(q => q.name === key)?.example || '00000000-0000-4000-8000-000000000000';
          segs.push(String(val));
        } else segs.push(seg);
      }
      real = segs.join('/');
      const q = {};
      for (const prm of op.parameters || []) {
        if (prm.in !== 'query') continue;
        if (prm.required || prm.example !== undefined || prm.schema?.example !== undefined) {
          q[prm.name] = prm.example ?? prm.schema?.example ?? (prm.schema?.enum ? prm.schema.enum[0] : '1');
        }
      }
      let body;
      const rb = op.requestBody?.content;
      if (rb) {
        const ct = Object.keys(rb)[0];
        if (ct.includes('json')) {
          const sch = rb[ct].schema;
          body = rb[ct].example ?? rb[ct].examples?.[Object.keys(rb[ct].examples ?? {})[0]]?.value ?? synthBody(sch);
        }
      }
      // NEVER start maintenance again — it bricks the API until the
      // immich_maintenance_token cookie (from the start response) is present.
      if (p === '/admin/maintenance' && m === 'post') body = { action: 'end' };
      const args = { query: q, body, token: TOKEN, note: 'generic coverage attempt (' + (op.operationId || '') + ')' };
      let rec = await call('coverage:' + (op.tags?.[0] || 'untagged'), method, real, args);
      if (p === '/auth/change-password' && m === 'post' && rec.status === 200 && body?.newPassword) PASSWORD = body.newPassword;
      if (rec.status === 401) {
        // change-password/logout invalidate sessions — re-login and retry once
        const rl = await call('auth', 'POST', '/auth/login', { body: { email: 'admin@immich.app', password: PASSWORD } });
        if (rl.status === 201 && rl.body?.accessToken) {
          TOKEN = rl.body.accessToken;
          args.token = TOKEN;
          rec = await call('coverage:' + (op.tags?.[0] || 'untagged'), method, real, args);
        }
      }
    }
  }

  // ---- write outputs
  const coverage = opCoverage();
  writeFileSync(`${OUT}/probe-results.json`, JSON.stringify(results, null, 1));
  writeFileSync(`${OUT}/dto-samples.json`, JSON.stringify(samples, null, 1));
  writeFileSync(`${OUT}/coverage.json`, JSON.stringify(coverage, null, 1));
  console.log('PROBE DONE. requests=', results.length, 'covered ops=', coverage.covered, '/', coverage.total);
}

function synthBody(sch, depth = 0) {
  if (!sch || depth > 4) return undefined;
  if (sch.example !== undefined) return sch.example;
  if (sch.default !== undefined) return sch.default;
  if (sch.$ref) {
    const name = sch.$ref.split('/').pop();
    return synthBody(spec.components?.schemas?.[name], depth + 1);
  }
  switch (sch.type) {
    case 'object': {
      const o = {};
      for (const [k, v] of Object.entries(sch.properties || {})) {
        const val = synthBody(v, depth + 1);
        if (val !== undefined) o[k] = val;
      }
      return o;
    }
    case 'array': return [];
    case 'string': return sch.enum ? sch.enum[0] : (sch.format === 'date-time' ? new Date().toISOString() : (sch.format === 'uuid' ? U() : 'probe'));
    case 'integer': case 'number': return 1;
    case 'boolean': return false;
    default: return undefined;
  }
}

function opCoverage() {
  const total = { total: 0, covered: 0, uncovered: [] };
  const have = new Set(results.map(r => r.method + ' ' + r.path));
  const cov = [];
  for (const [p, ms] of Object.entries(spec.paths)) {
    for (const [m, op] of Object.entries(ms)) {
      if (!['get', 'post', 'put', 'patch', 'delete'].includes(m)) continue;
      total.total++;
      const method = m.toUpperCase();
      const ps = p.split('/');
      const hit = results.some(r => {
        if (r.method !== method || r.status === 0) return false;
        const segs = r.path.split('?')[0].split('/');
        if (segs.length !== ps.length) return false;
        return ps.every((s, i) => s.startsWith('{') || s === segs[i]);
      });
      if (hit) total.covered++; else total.uncovered.push(`${method} ${p}`);
      cov.push({ method, path: p, operationId: op.operationId, covered: hit });
    }
  }
  total.byOperation = cov;
  return total;
}

main().catch(e => { console.error('PROBE FAILED', e); process.exit(1); });
