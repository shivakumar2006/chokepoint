export default function StatCard({ label, value, tone = 'slate', sub }) {
  const tones = {
    slate: 'text-slate-100', red: 'text-rose-400', green: 'text-emerald-400', amber: 'text-amber-400', cyan: 'text-cyan-300',
  }
  return (
    <div className="glass rounded-xl px-4 py-3">
      <div className={`text-2xl font-bold tabular-nums ${tones[tone]}`}>{value}</div>
      <div className="text-[11px] uppercase tracking-wider text-slate-400">{label}</div>
      {sub && <div className="mt-0.5 text-[11px] text-slate-500">{sub}</div>}
    </div>
  )
}