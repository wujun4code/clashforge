package workernode

// VlessWorkerScript is the Cloudflare Worker JS deployed as a VLESS-over-WebSocket proxy.
//
// Env bindings (all plain_text):
//   UUID       — VLESS UUID for auth (required)
//   EXPIRES_AT — ISO 8601 timestamp; requests after this time return 410 (optional)
//   AES_KEY    — 32-byte hex key; enables the GET /sub subscription endpoint (optional)
const VlessWorkerScript = `
import { connect } from 'cloudflare:sockets';

export default {
  async fetch(request, env) {
    // Expiry gate: applies to all requests when EXPIRES_AT is set.
    if (env.EXPIRES_AT && Date.now() > new Date(env.EXPIRES_AT).getTime()) {
      return Response.json(
        { error: 'expired', expires_at: env.EXPIRES_AT },
        { status: 410 }
      );
    }

    const url = new URL(request.url);

    // Subscription endpoint: GET /sub — returns AES-GCM encrypted Clash proxy YAML.
    if (request.method === 'GET' && url.pathname === '/sub') {
      if (!env.AES_KEY) {
        return new Response('subscription not configured\n', { status: 404 });
      }
      const hostname = url.hostname;
      const uuid = (env.UUID || '').toLowerCase().replace(/-/g, '');
      const yaml = buildClashProxyYaml(hostname, uuid);
      const encrypted = await aesGcmEncrypt(yaml, env.AES_KEY);
      return new Response(encrypted, {
        headers: {
          'Content-Type': 'application/octet-stream',
          'X-Expires': env.EXPIRES_AT || '',
        },
      });
    }

    // VLESS WebSocket proxy.
    if (request.headers.get('Upgrade') !== 'websocket') {
      return new Response('ClashForge Proxy Node\n', { status: 200 });
    }
    const uuid = (env.UUID || '').toLowerCase().replace(/-/g, '');
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    server.accept();
    proxyVless(server, uuid).catch(() => {});
    return new Response(null, { status: 101, webSocket: client });
  }
};

// buildClashProxyYaml returns a Clash-compatible proxies YAML fragment.
function buildClashProxyYaml(hostname, uuid) {
  return [
    'proxies:',
    '- name: ClashForge Free',
    '  type: vless',
    '  server: ' + hostname,
    '  port: 443',
    '  uuid: ' + uuid,
    '  tls: true',
    '  network: ws',
    '  ws-opts:',
    '    path: /',
    '    headers:',
    '      Host: ' + hostname,
    '  udp: false',
    '',
  ].join('\n');
}

// aesGcmEncrypt encrypts plaintext with AES-256-GCM using the WebCrypto API.
// keyHex must be a 64-char hex string (32 bytes). Output format: IV(12) || ciphertext.
async function aesGcmEncrypt(plaintext, keyHex) {
  const keyBytes = hexToBytes(keyHex);
  const key = await crypto.subtle.importKey('raw', keyBytes, 'AES-GCM', false, ['encrypt']);
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const data = new TextEncoder().encode(plaintext);
  const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, data);
  const result = new Uint8Array(12 + ciphertext.byteLength);
  result.set(iv);
  result.set(new Uint8Array(ciphertext), 12);
  return result.buffer;
}

function hexToBytes(hex) {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
}

async function proxyVless(ws, uuid) {
  let buf = new Uint8Array(0);
  let writer = null;
  let ready = false;

  ws.addEventListener('message', async ({ data }) => {
    const chunk = toBytes(data);
    if (!ready) {
      buf = concat(buf, chunk);
      const r = parseHeader(buf, uuid);
      if (r === null) return;
      if (r.error) { ws.close(1002, r.error); return; }
      ready = true;
      buf = new Uint8Array(0);
      try {
        const remote = connect({ hostname: r.host, port: r.port });
        writer = remote.writable.getWriter();
        ws.send(new Uint8Array([0, 0]).buffer);
        if (r.payload.length > 0) await writer.write(r.payload);
        pipeToWs(remote, ws);
      } catch (e) { ws.close(1011, String(e)); }
    } else if (writer) {
      try { await writer.write(chunk); } catch (e) { ws.close(1011, String(e)); }
    }
  });

  ws.addEventListener('close', () => { try { writer?.close(); } catch {} });
  ws.addEventListener('error', () => { try { writer?.close(); } catch {} });
}

function pipeToWs(remote, ws) {
  const reader = remote.readable.getReader();
  (async () => {
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        ws.send(value);
      }
    } catch {}
    try { ws.close(1000); } catch {}
  })();
}

function parseHeader(buf, uuid) {
  if (buf.length < 24) return null;
  if (buf[0] !== 0) return { error: 'unsupported version' };
  const got = hexOf(buf.slice(1, 17));
  if (uuid && got !== uuid) return { error: 'invalid uuid' };
  const addonLen = buf[17];
  const base = 18 + addonLen;
  if (buf.length < base + 4) return null;
  if (buf[base] !== 1) return { error: 'only TCP (cmd=1) is supported' };
  const port = (buf[base + 1] << 8) | buf[base + 2];
  const atype = buf[base + 3];
  let cur = base + 4;
  let host;
  if (atype === 1) {
    if (buf.length < cur + 4) return null;
    host = [...buf.slice(cur, cur + 4)].join('.');
    cur += 4;
  } else if (atype === 2) {
    if (buf.length < cur + 1) return null;
    const len = buf[cur++];
    if (buf.length < cur + len) return null;
    host = new TextDecoder().decode(buf.slice(cur, cur + len));
    cur += len;
  } else if (atype === 3) {
    if (buf.length < cur + 16) return null;
    const parts = [];
    for (let i = 0; i < 8; i++)
      parts.push(((buf[cur + i * 2] << 8) | buf[cur + i * 2 + 1]).toString(16).padStart(4, '0'));
    host = parts.join(':');
    cur += 16;
  } else return { error: 'unknown address type' };
  return { host, port, payload: buf.slice(cur) };
}

function toBytes(data) {
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  if (typeof data === 'string') return new TextEncoder().encode(data);
  return new Uint8Array(data);
}

function concat(a, b) {
  const c = new Uint8Array(a.length + b.length);
  c.set(a); c.set(b, a.length);
  return c;
}

function hexOf(bytes) {
  return [...bytes].map(b => b.toString(16).padStart(2, '0')).join('');
}
`
