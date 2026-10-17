import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowRight, Play, RotateCcw, Loader2, Network, Waypoints, Bot, Scissors } from 'lucide-react'
import { api, API_URL } from '../lib/api.js'
import GraphView from '../components/GraphView.jsx'
import StatCard from '../components/Statcard.jsx'
import FixPanel from '../components/Fixpanel.jsx'
import PathsPanel from '../components/Pathspanel.jsx'
import AgentLog from '../components/Agentslog.jsx'

const TABS = [
  { id: 'fix', label: 'Fixes', icon: Scissors },
  { id: 'paths', label: 'Proof chains', icon: Waypoints },
  { id: 'agent', label: 'Agent', icon: Bot },
]

export default function Console() {
  const [source, setSource] = useState('intern')
  const [graph, setGraph] = useState(null)
  const [mincut, setMincut] = useState(null)
  const [selIds, setSelIds] = useState(null)
  const [impact, setImpact] = useState(null)
  const [hiPath, setHiPath] = useState(null)
  const [tab, setTab] = useState('fix')
  const [trace, setTrace] = useState([])
  const [running, setRunning] = useState(false)
  const [online, setOnline] = useState(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const lastLen = useRef(-1)
  const sourceRef = useRef(source)
  sourceRef.current = source

  const refresh = useCallback(async () => {
    try {
      setError('')
      const src = sourceRef.current
      const [g, m] = await Promise.all([api.graph(src), api.mincut(src)])
      setGraph(g); setMincut(m); setSelIds(null); setImpact(null); setHiPath(null)
    } catch (e) { setError(e.message) }
  }, [])

  useEffect(() => { refresh() }, [source, refresh])

  // health + agent trace polling
  useEffect(() => {
    let alive = true
    const tick = async () => {
      try {
        const [t, s] = await Promise.all([api.trace(), api.status()])
        if (!alive) return
        setOnline(true); setRunning(s.running)
        if (t.length !== lastLen.current) {
          const hadApply = t.some((e) => e.type === 'tool_result' && e.name === 'apply_fix')
          const first = lastLen.current === -1
          lastLen.current = t.length; setTrace(t)
          if (hadApply && !first) refresh()
        }
      } catch { if (alive) setOnline(false) }
    }
    tick()
    const id = setInterval(tick, 1500)
    return () => { alive = false; clearInterval(id) }
  }, [refresh])

  async function select(ids) {
    setSelIds(ids); setImpact(null)
    try { setImpact(await api.simulate(source, ids)) } catch (e) { setError(e.message) }
  }
  async function run() {
    setBusy('run'); setTab('agent'); lastLen.current = -1; setTrace([])
    try { setError(''); await api.run(source) } catch (e) { setError(e.message) } finally { setBusy('') }
  }
  async function reset() {
    setBusy('reset')
    try { setError(''); await api.reset(); lastLen.current = -1; setTrace([]); await refresh() } catch (e) { setError(e.message) } finally { setBusy('') }
  }

  const left = graph ? (selIds ? graph.paths.filter((p) => !p.permission_ids.some((id) => selIds.includes(id))).length : graph.paths.length) : 0
  const total = graph?.paths.length ?? 0
  const exposed = graph ? new Set(graph.paths.map((p) => p.target)).size : 0

  return (
    <div className="grid-bg flex h-full min-h-0 flex-col">
      <header className="glass z-10 flex flex-wrap items-center gap-3 border-x-0 border-t-0 px-4 py-2.5">
        <a href="#/" className="mr-2 flex items-center gap-2 font-bold tracking-tight"><span className="text-xl">⛓</span> Chokepoint</a>
        <label className="text-xs uppercase tracking-wider text-slate-400">Attacker</label>
        <select value={source} onChange={(e) => setSource(e.target.value)}
          className="rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm text-slate-100 outline-none focus:border-cyan-400">
          {(graph?.users || [source]).map((u) => <option key={u} value={u}>{u}</option>)}
        </select>
        <span className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] ${online ? 'bg-emerald-500/15 text-emerald-300' : online === false ? 'bg-rose-500/15 text-rose-300' : 'bg-slate-700/40 text-slate-400'}`}>
          <i className={`h-1.5 w-1.5 rounded-full ${online ? 'bg-emerald-400' : online === false ? 'bg-rose-400' : 'bg-slate-500'}`} />
          {online ? 'API online' : online === false ? 'API offline' : 'connecting'} <span className="hidden text-slate-500 sm:inline">· {API_URL}</span>
        </span>
        <span className="flex-1" />
        <button onClick={reset} disabled={busy !== '' || running}
          className="flex items-center gap-2 rounded-lg border border-slate-700 px-3 py-1.5 text-sm text-slate-200 hover:border-slate-500 disabled:opacity-50">
          {busy === 'reset' ? <Loader2 size={14} className="animate-spin" /> : <RotateCcw size={14} />} Reset graph
        </button>
        <button onClick={run} disabled={busy !== '' || running || !online}
          className="flex items-center gap-2 rounded-lg bg-gradient-to-r from-cyan-500 to-violet-500 px-4 py-1.5 text-sm font-semibold text-slate-950 hover:brightness-110 disabled:opacity-50">
          {running ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />} {running ? 'Agent running…' : 'Run agent'}
        </button>
      </header>

      {error && (
        <div className="mx-4 mt-3 rounded-lg border border-rose-500/40 bg-rose-500/10 px-4 py-2 text-sm text-rose-200">⚠ {error}</div>
      )}

      <main className="grid min-h-0 flex-1 gap-3 p-3 lg:grid-cols-[1.5fr_1fr]">
        <section className="glass relative flex min-h-[420px] flex-col overflow-hidden rounded-2xl">
          <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-4">
            <StatCard label="attack paths" value={`${left}${selIds ? ` / ${total}` : ''}`} tone={left ? 'red' : 'green'} sub={selIds ? 'after selected fix' : 'to crown jewels'} />
            <StatCard label="crown jewels exposed" value={exposed} tone={exposed ? 'red' : 'green'} sub={graph ? `of ${graph.crown_jewels.length}` : ''} />
            <StatCard label="min-cut size" value={mincut?.min_cut_size || '—'} tone="cyan" sub="permissions to remove" />
            <StatCard label="legit broken" value={impact ? impact.legit_access_broken.length : '—'} tone={impact?.legit_access_broken.length ? 'red' : 'green'} sub="for selected fix" />
          </div>
          <div className="min-h-0 flex-1">
            {graph ? <GraphView graph={graph} source={source} selIds={selIds} hiPath={hiPath} /> :
              <div className="flex h-full items-center justify-center text-slate-500"><Network className="mr-2" /> loading graph…</div>}
          </div>
        </section>

        <section className="glass flex min-h-[420px] flex-col overflow-hidden rounded-2xl">
          <div className="flex border-b border-slate-700/60">
            {TABS.map(({ id, label, icon: Icon }) => (
              <button key={id} onClick={() => setTab(id)}
                className={`flex flex-1 items-center justify-center gap-2 px-3 py-3 text-sm transition ${tab === id ? 'border-b-2 border-cyan-400 text-cyan-300' : 'text-slate-400 hover:text-slate-200'}`}>
                <Icon size={15} /> {label}
                {id === 'agent' && running && <i className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400" />}
              </button>
            ))}
          </div>
          <div className="scroll-thin min-h-0 flex-1 overflow-auto p-4">
            {tab === 'fix' && <FixPanel source={source} mincut={mincut} selIds={selIds} onSelect={select} impact={impact} onApplied={refresh} onError={(e) => setError(e.message)} />}
            {tab === 'paths' && <PathsPanel graph={graph} hiPath={hiPath} onHighlight={setHiPath} />}
            {tab === 'agent' && <AgentLog trace={trace} running={running} />}
          </div>
        </section>
      </main>

      <footer className="px-4 pb-2 text-center text-[11px] text-slate-600">
        Explicit-allow model · 2 privilege-escalation patterns · legit access = labeled set in the demo data <ArrowRight size={10} className="inline" /> <a className="underline hover:text-slate-400" href="#/">about</a>
      </footer>
    </div>
  )
}