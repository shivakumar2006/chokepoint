import { useState } from 'react'
import { Scissors, ShieldCheck, TriangleAlert, Loader2, CircleCheck } from 'lucide-react'
import { api } from '../lib/api.js'

function Tag({ tone, children }) {
  const t = {
    ok: 'bg-emerald-500/15 text-emerald-300', no: 'bg-rose-500/15 text-rose-300', min: 'bg-cyan-500/15 text-cyan-300',
  }[tone]
  return <span className={`mr-1.5 inline-block rounded-full px-2 py-0.5 text-[10px] font-semibold ${t}`}>{children}</span>
}

function OptionCard({ im, label, min, active, onClick }) {
  const safe = im.attacker_contained && !im.legit_access_broken.length
  return (
    <button onClick={onClick}
      className={`w-full rounded-lg border p-3 text-left transition ${active ? 'border-emerald-400 bg-emerald-500/5 ring-1 ring-emerald-400/40' : 'border-slate-700/70 bg-slate-900/40 hover:border-slate-500'} ${safe ? '' : 'opacity-75'}`}>
      <div className="mb-1.5 flex flex-wrap items-center">
        <b className="mr-2 text-sm text-slate-100">{label}</b>
        {min && <Tag tone="min">MIN CUT</Tag>}
        {im.attacker_contained ? <Tag tone="ok">contains attacker</Tag> : <Tag tone="no">{im.still_reachable_crown_jewels.length} crown jewel(s) still reachable</Tag>}
        {im.legit_access_broken.length > 0 && <Tag tone="no">breaks legit access</Tag>}
      </div>
      {im.remove.map((r, i) => <div key={i} className="font-mono text-[12px] text-slate-300">remove {r}</div>)}
      <div className="mt-1.5 text-[11px] text-slate-500">
        kills {im.attack_paths_killed}/{im.attack_paths_total} paths · {im.collateral_permissions_lost.length} permissions lost by legit users
      </div>
    </button>
  )
}

export default function FixPanel({ source, mincut, selIds, onSelect, impact, onApplied, onError }) {
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState(null)

  if (!mincut) return <p className="text-sm text-slate-500">Loading…</p>
  if (!mincut.attack_paths_total) {
    return (
      <div className="space-y-3">
        {result && <ApplyResult r={result} />}
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-emerald-300">
          No attack paths from <b>{source}</b>. Nothing to fix.
        </div>
      </div>
    )
  }

  const minKeys = new Set(mincut.min_cut_options.map((o) => o.remove_ids.join(',')))
  const others = mincut.best_single_fixes.filter((s) => !minKeys.has(s.remove_ids.join(','))).slice(0, 5)
  const keyOf = (im) => im.remove_ids.join(',')
  const isActive = (im) => selIds && keyOf(im) === selIds.join(',')
  const canApply = impact && impact.attacker_contained && !impact.legit_access_broken.length

  async function apply() {
    setBusy(true)
    try {
      const r = await api.apply(source, selIds)
      setResult(r); setConfirm(false)
      onApplied()
    } catch (e) { onError(e); setConfirm(false) } finally { setBusy(false) }
  }

  return (
    <div className="space-y-3">
      {result && <ApplyResult r={result} />}
      <div className="text-xs text-slate-400">
        Minimum set of permissions that closes <b className="text-slate-200">all {mincut.attack_paths_total}</b> attack paths while every legit access keeps working. Pick one to see the counterfactual.
      </div>
      {mincut.min_cut_options.map((o, i) => (
        <OptionCard key={keyOf(o)} im={o} min label={`Option ${String.fromCharCode(65 + i)}`} active={isActive(o)} onClick={() => onSelect(o.remove_ids)} />
      ))}
      {others.length > 0 && <div className="pt-1 text-[11px] uppercase tracking-wider text-slate-500">Other single fixes (insufficient or unsafe)</div>}
      {others.map((o) => <OptionCard key={keyOf(o)} im={o} label={o.remove_ids[0]} active={isActive(o)} onClick={() => onSelect(o.remove_ids)} />)}
      {mincut.rejected_because_legit_breaks.length > 0 && (
        <>
          <div className="pt-1 text-[11px] uppercase tracking-wider text-slate-500">Rejected — would break legit access</div>
          {mincut.rejected_because_legit_breaks.map((o) => <OptionCard key={keyOf(o)} im={o} label="rejected" active={isActive(o)} onClick={() => onSelect(o.remove_ids)} />)}
        </>
      )}

      {impact && (
        <div className="glass rounded-xl p-4">
          <div className="grid grid-cols-3 gap-3 text-center">
            <div><div className="text-xl font-bold text-slate-100">{impact.attack_paths_killed}/{impact.attack_paths_total}</div><div className="text-[10px] uppercase text-slate-500">paths killed</div></div>
            <div><div className={`text-xl font-bold ${impact.legit_access_broken.length ? 'text-rose-400' : 'text-emerald-400'}`}>{impact.legit_access_broken.length}</div><div className="text-[10px] uppercase text-slate-500">legit broken</div></div>
            <div><div className="text-xl font-bold text-amber-300">{impact.collateral_permissions_lost.length}</div><div className="text-[10px] uppercase text-slate-500">perms lost</div></div>
          </div>
          {impact.collateral_permissions_lost.length > 0 && (
            <div className="scroll-thin mt-3 max-h-28 overflow-auto font-mono text-[11px] text-slate-400">
              {impact.collateral_permissions_lost.map((c, i) => <div key={i}>− {c}</div>)}
            </div>
          )}
          <button disabled={!canApply} onClick={() => setConfirm(true)}
            className="mt-3 flex w-full items-center justify-center gap-2 rounded-lg bg-emerald-500 px-4 py-2.5 text-sm font-semibold text-slate-950 transition hover:bg-emerald-400 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-400">
            <Scissors size={16} /> {canApply ? 'Apply this fix…' : 'Not safe to apply'}
          </button>
        </div>
      )}

      {confirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" onClick={() => !busy && setConfirm(false)}>
          <div className="glass w-full max-w-md rounded-2xl p-6" onClick={(e) => e.stopPropagation()}>
            <div className="mb-3 flex items-center gap-2 text-amber-300"><TriangleAlert size={18} /><b>Human approval</b></div>
            <p className="mb-3 text-sm text-slate-300">This removes the following from the graph, regenerates <code className="text-slate-100">out/remediation.sh</code> and re-runs the attack-path query to verify:</p>
            <div className="mb-4 space-y-1 rounded-lg bg-slate-950/70 p-3 font-mono text-[12px] text-slate-200">
              {impact.remove.map((r, i) => <div key={i}>− {r}</div>)}
            </div>
            <div className="flex justify-end gap-2">
              <button disabled={busy} onClick={() => setConfirm(false)} className="rounded-lg border border-slate-600 px-4 py-2 text-sm text-slate-300 hover:border-slate-400">Cancel</button>
              <button disabled={busy} onClick={apply} className="flex items-center gap-2 rounded-lg bg-emerald-500 px-4 py-2 text-sm font-semibold text-slate-950 hover:bg-emerald-400">
                {busy && <Loader2 size={14} className="animate-spin" />} Approve & apply
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function ApplyResult({ r }) {
  return (
    <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4">
      <div className="mb-2 flex items-center gap-2 font-semibold text-emerald-300"><ShieldCheck size={18} /> Fix applied and verified from the graph</div>
      <div className="mb-3 flex items-center gap-3 text-2xl font-bold">
        <span className="text-rose-400">{r.attack_paths_before}</span><span className="text-slate-500">→</span><span className="text-emerald-400">{r.attack_paths_after}</span>
        <span className="text-xs font-normal text-slate-400">attack paths</span>
        {r.verified_from_graph && <CircleCheck size={18} className="text-emerald-400" />}
      </div>
      <div className="mb-1 text-[11px] uppercase tracking-wider text-slate-400">{r.remediation_file}</div>
      <pre className="scroll-thin overflow-auto rounded-md bg-slate-950/70 p-3 font-mono text-[11px] text-slate-200">{r.remediation_commands.join('\n')}</pre>
    </div>
  )
}