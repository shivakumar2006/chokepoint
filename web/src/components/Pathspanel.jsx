import { ChevronRight } from 'lucide-react'

const typeTone = {
  MEMBER_OF: 'text-violet-300', HAS_POLICY: 'text-slate-300', CAN_ASSUME: 'text-amber-300',
  ALLOWS: 'text-emerald-300', ESCALATES: 'text-rose-300',
}

// "Proof chain": every hop of every attack path, with the permission edges that make it possible.
export default function PathsPanel({ graph, hiPath, onHighlight }) {
  if (!graph) return null
  if (!graph.paths.length) {
    return <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-emerald-300">No attack path from <b>{graph.source}</b> to any crown jewel.</div>
  }
  return (
    <div className="space-y-2">
      <p className="text-xs text-slate-400">Click a path to isolate it on the graph. Each hop is a real edge in FalkorDB.</p>
      {graph.paths.map((p, i) => (
        <button key={i} onClick={() => onHighlight(hiPath === i ? null : i)}
          className={`w-full rounded-lg border p-3 text-left transition ${hiPath === i ? 'border-rose-400 bg-rose-500/10' : 'border-slate-700/70 bg-slate-900/40 hover:border-slate-500'}`}>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-xs font-semibold text-rose-300">Path #{i + 1} → {p.target}</span>
            <span className="text-[11px] text-slate-500">{p.edge_types.length} hops</span>
          </div>
          <ol className="space-y-1 font-mono text-[12px]">
            {p.edge_types.map((t, k) => (
              <li key={k} className="flex flex-wrap items-center gap-1.5">
                <span className="text-slate-500">{k + 1}.</span>
                <span className="text-slate-200">{p.nodes[k]}</span>
                <span className={`flex items-center ${typeTone[t] || 'text-slate-300'}`}><ChevronRight size={12} />{t}<ChevronRight size={12} /></span>
                <span className="text-slate-200">{p.nodes[k + 1]}</span>
              </li>
            ))}
          </ol>
          <div className="mt-2 text-[11px] text-slate-500">depends on permission edges: <span className="font-mono text-slate-400">{p.permission_ids.join(', ')}</span></div>
        </button>
      ))}
    </div>
  )
}