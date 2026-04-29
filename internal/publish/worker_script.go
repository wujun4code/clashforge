package publish

const WorkerScriptSource = `let mytoken = 'passwd';

export default {
  async fetch(request, env) {
    try {
      mytoken = env.TOKEN || mytoken;
      if (!env.KV) throw new Error('KV namespace not bound');

      const url = new URL(request.url);
      const pathKey = url.pathname.length > 1 ? url.pathname.slice(1) : '';
      const token = (pathKey === mytoken)
        ? mytoken
        : (url.searchParams.get('token') || 'null');

      if (token !== mytoken) return makeResp('Unauthorized', 403);
      if (!pathKey || pathKey === mytoken) return makeResp('clashforge worker OK', 200);

      const shouldDelete = request.method === 'DELETE' || url.searchParams.get('delete') === '1';
      if (shouldDelete) {
        await env.KV.delete(pathKey);
        return makeResp('Deleted', 200);
      }

      if (request.method === 'POST' || request.method === 'PUT') {
        const raw = await request.arrayBuffer();
        const content = new TextDecoder('utf-8').decode(raw);
        await env.KV.put(pathKey, content);
        return makeResp(content);
      }

      return await fileOp(env.KV, pathKey, url);
    } catch (err) {
      return makeResp('Error: ' + err.message, 500);
    }
  }
};

async function fileOp(KV, key, url) {
  const text = url.searchParams.get('text');
  const b64  = url.searchParams.get('b64');

  if (!text && !b64) {
    const val = await KV.get(key, { cacheTtl: 60 });
    return val === null ? makeResp('Not found', 404) : makeResp(val);
  }

  let content;
  if (text) {
    content = text;
  } else {
    const raw = b64.replace(/ /g, '+');
    const bytes = new Uint8Array(atob(raw).split('').map(c => c.charCodeAt(0)));
    content = new TextDecoder('utf-8').decode(bytes);
  }

  await KV.put(key, content);
  return makeResp(content);
}

function makeResp(body, status, extra) {
  status = status || 200;
  extra  = extra || {};

  return new Response(body, {
    status,
    headers: Object.assign({
      'Content-Type': 'text/plain; charset=utf-8',
      'Cache-Control': 'no-store, no-cache, must-revalidate',
      'Pragma': 'no-cache',
      'Expires': '0'
    }, extra)
  });
}
`
