// All calls go straight to the Go API (CORS enabled there).
export const API_URL = (import.meta.env.VITE_API_URL || 'http://localhost:8080').replace(/\/$/, '')

async function req(path, opts = {}) {
  let res
  try {
    res = await fetch(API_URL + path, {
      ...opts,
      headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
    })
  } catch {
    throw new Error(`Cannot reach the API at ${API_URL}. Is "go run ./server" running?`)
  }
  const text = await res.text()
  let data = null
  try { data = JSON.parse(text) } catch { /* not json */ }
  if (!res.ok) throw new Error((data && data.error) || text || `HTTP ${res.status}`)
  return data
}

const post = (path, body) => req(path, { method: 'POST', body: JSON.stringify(body || {}) })
const q = (s) => encodeURIComponent(s)

export const api = {
  health: () => req('/api/health'),
  graph: (source) => req(`/api/graph?source=${q(source)}`),
  mincut: (source) => req(`/api/mincut?source=${q(source)}`),
  simulate: (source, ids) => req(`/api/simulate?source=${q(source)}&ids=${q(ids.join(','))}`),
  apply: (source, ids) => post('/api/apply', { source, ids }),
  remediation: () => req('/api/remediation'),
  trace: () => req('/api/trace'),
  status: () => req('/api/status'),
  run: (source, task) => post('/api/run', { source, task }),
  reset: () => post('/api/reset'),
}