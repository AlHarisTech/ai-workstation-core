import http from 'http';
import { spawn } from 'child_process';
import { randomUUID } from 'crypto';
import { URL } from 'url';

const PORT = parseInt(process.env.MCP_PORT || '3000');
const CMD = process.env.MCP_CMD;
const CMD_ARGS = process.env.MCP_ARGS ? JSON.parse(process.env.MCP_ARGS) : [];

if (!CMD) {
  console.error('FATAL: MCP_CMD environment variable is required');
  process.exit(1);
}

let proc;
let initPromise;
let initResolve;
let toolsCache = null;
let sessions = new Map();
let respQueue = [];

function startProc() {
  proc = spawn(CMD, CMD_ARGS, {
    stdio: ['pipe', 'pipe', 'pipe'],
    env: { ...process.env, NODE_ENV: 'production' },
  });

  let buf = '';
  proc.stdout.on('data', (data) => {
    buf += data.toString();
    processBuf();
  });

  function processBuf() {
    while (true) {
      const nl = buf.indexOf('\n');
      if (nl < 0) break;
      const line = buf.slice(0, nl).trim();
      buf = buf.slice(nl + 1);
      if (!line) continue;
      try {
        const msg = JSON.parse(line);
        if (respQueue.length > 0) {
          const resolve = respQueue.shift();
          resolve(msg);
        }
      } catch { /* not json */ }
    }
  }

  proc.stderr.on('data', (data) => {
    for (const line of data.toString().split('\n').filter(l => l.trim())) {
      console.error(`[sub] ${line}`);
    }
  });

  proc.on('exit', (code, sig) => {
    console.error(`subprocess exited code=${code} signal=${sig} — restarting`);
    setTimeout(startProc, 1000);
  });

  proc.on('error', (err) => {
    console.error(`subprocess error: ${err.message} — restarting`);
    setTimeout(startProc, 1000);
  });

  // Initialize server
  initPromise = new Promise((resolve) => { initResolve = resolve; });
  toolsCache = null;

  send({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'mcp-sse-proxy', version: '1.0' } } })
    .then(() => send({ jsonrpc: '2.0', id: 2, method: 'notifications/initialized', params: {} }))
    .then(() => send({ jsonrpc: '2.0', id: 3, method: 'tools/list', params: {} }))
    .then((resp) => {
      if (resp.result && resp.result.tools) {
        toolsCache = resp.result.tools;
      }
      console.error(`initialized with ${toolsCache ? toolsCache.length : 0} tools`);
      initResolve();
    });
}

function send(msg) {
  return new Promise((resolve) => {
    respQueue.push(resolve);
    proc.stdin.write(JSON.stringify(msg) + '\n');
  });
}

function json(res, code, data) {
  res.writeHead(code, { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' });
  res.end(JSON.stringify(data));
}

const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${req.headers.host || 'localhost'}`);
  const method = req.method;
  const ts = Date.now();

  const logReq = (extra = '') => {
    const elapsed = Date.now() - ts;
    console.error(`[${method} ${url.pathname}] accept=${(req.headers.accept || '').slice(0,40)} extra=${extra} elapsed=${elapsed}ms`);
  };

  // Health
  if (url.pathname === '/health') {
    logReq();
    return json(res, 200, { status: 'ok', port: PORT, cmd: CMD, tools: toolsCache ? toolsCache.length : 0 });
  }

  // SSE mode
  if ((url.pathname === '/' || url.pathname === '/sse') && method === 'GET') {
    const accept = req.headers.accept || '';
    if (accept.includes('text/event-stream') || url.pathname === '/sse') {
      logReq('SSE');
      return handleSSE(req, res);
    }
  }

  // Direct POST JSON-RPC
  if (method === 'POST') {
    if (url.pathname === '/messages') {
      logReq('messages');
      return handleMessages(req, res, url);
    }
    if (url.pathname === '/') {
      logReq('direct-POST');
      return handleDirectPost(req, res);
    }
  }

  // GET / without SSE accept — server info
  if (url.pathname === '/' && method === 'GET') {
    logReq('info');
    return json(res, 200, { server: CMD.split('/').pop(), status: 'ok', port: PORT });
  }

  logReq('404');
  json(res, 404, { error: 'not found' });
});

function handleDirectPost(req, res) {
  let body = '';
  req.on('data', (chunk) => { body += chunk; });
  req.on('end', async () => {
    try {
      const msg = JSON.parse(body);
      const { method, params, id } = msg;
      console.error(`[direct] method=${method} id=${id}`);

      if (method === 'initialize') {
        return json(res, 200, {
          jsonrpc: '2.0', id,
          result: { protocolVersion: '2024-11-05', capabilities: { tools: {} }, serverInfo: { name: CMD.split('/').pop(), version: '1.0' } },
        });
      }

      if (method === 'tools/list') {
        return json(res, 200, {
          jsonrpc: '2.0', id,
          result: { tools: toolsCache || [] },
        });
      }

      // Notifications (no id) — forward but don't wait for response
      if (id === undefined || id === null) {
        if (initPromise) await initPromise;
        proc.stdin.write(JSON.stringify(msg) + '\n');
        return json(res, 200, { jsonrpc: '2.0' });
      }

      // Wait for subprocess initialization
      if (initPromise) await initPromise;

      // Forward to subprocess
      const resp = await send(msg);
      return json(res, 200, resp);
    } catch (e) {
      return json(res, 400, { jsonrpc: '2.0', error: { code: -32700, message: 'parse error' } });
    }
  });
}

function handleMessages(req, res, url) {
  const sessionId = url.searchParams.get('sessionId');
  const session = sessions.get(sessionId);
  if (!session) return json(res, 404, { error: 'session not found' });
  let body = '';
  req.on('data', (chunk) => { body += chunk; });
  req.on('end', () => {
    try {
      session.proc.stdin.write(JSON.stringify(JSON.parse(body)) + '\n');
      json(res, 200, { ok: true });
    } catch {
      json(res, 400, { error: 'invalid json' });
    }
  });
}

function handleSSE(req, res) {
  const sessionId = randomUUID();

  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    'Connection': 'keep-alive',
    'Access-Control-Allow-Origin': '*',
    'X-Accel-Buffering': 'no',
  });

  const sProc = spawn(CMD, CMD_ARGS, {
    stdio: ['pipe', 'pipe', 'pipe'],
    env: { ...process.env, NODE_ENV: 'production' },
  });

  sessions.set(sessionId, { proc: sProc, res });

  const log = (msg) => console.error(`[${sessionId.slice(0, 8)}] ${msg}`);

  sProc.stdout.on('data', (data) => {
    const lines = data.toString().split('\n').filter((l) => l.trim());
    for (const line of lines) {
      try { JSON.parse(line); } catch { log(`non-json: ${line.slice(0, 200)}`); continue; }
      res.write(`event: message\ndata: ${line}\n\n`);
    }
  });

  sProc.stderr.on('data', (data) => {
    for (const line of data.toString().split('\n').filter((l) => l.trim())) log(line);
  });

  sProc.on('exit', (code, sig) => {
    log(`exited code=${code} signal=${sig}`);
    try { res.end(); } catch {}
    sessions.delete(sessionId);
  });

  sProc.on('error', (err) => {
    log(`spawn error: ${err.message}`);
    try { res.end(); } catch {}
    sessions.delete(sessionId);
  });

  req.on('close', () => {
    log('disconnected');
    sProc.kill();
    sessions.delete(sessionId);
  });

  res.write(`event: endpoint\ndata: /messages?sessionId=${sessionId}\n\n`);
}

startProc();
server.listen(PORT, () => {
  console.error(`MCP proxy ready on port ${PORT}: ${CMD} ${CMD_ARGS.join(' ')}`);
});
