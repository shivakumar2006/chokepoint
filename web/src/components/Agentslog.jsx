import { useEffect, useRef } from 'react'
import { Bot, Terminal, CheckCircle2, XCircle } from 'lucide-react'

const short = (s, n) => { s = String(s).replace(/\s+/g, ' '); return s.length > n ? s.slice(0, n) + ' …' : s }

export default function AgentLog({ trace, running }) {
  const end = useRef(null)
  useEffect(() => { end.current?.scrollIntoView({ block: 'end' }) }, [trace.length])

  if (!trace.length) {
    return <p className="text-sm text-slate-500">{running ? 'Agent is starting…' : 'No run yet. Press “Run agent” and watch it pick tools from the graph.'}</p>
  }
  return (
    <div className="space-y-2.5 text-[13px]">
      {trace.map((e, i) => {
        const time = new Date(e.t).toLocaleTimeString()
        if (e.type === 'task') return <div key={i} className="rounded-md border border-slate-700 bg-slate-900/60 p-2 text-slate-300"><b className="text-cyan-300">TASK</b> {e.data}</div>
        if (e.type === 'text') return (
          <div key={i} className="flex gap-2">
            <Bot size={15} className="mt-0.5 shrink-0 text-violet-300" />
            <div><span className="text-[10px] text-slate-500">{time}</span><div className="whitespace-pre-wrap text-slate-200">{e.data}</div></div>
          </div>)
        if (e.type === 'tool_call') return (
          <div key={i} className="flex gap-2 font-mono text-[12px] text-cyan-300">
            <Terminal size={14} className="mt-0.5 shrink-0" /><span><b>{e.name}</b> <span className="text-cyan-300/60">{short(JSON.stringify(e.data), 120)}</span></span>
          </div>)
        if (e.type === 'tool_result') return <div key={i} className="ml-5 border-l border-slate-700 pl-2 font-mono text-[11px] text-slate-500">{short(e.data, 220)}</div>
        if (e.type === 'approval') return (
          <div key={i} className={`flex items-center gap-2 text-xs ${e.data ? 'text-emerald-300' : 'text-rose-300'}`}>
            {e.data ? <CheckCircle2 size={14} /> : <XCircle size={14} />} human approval: {e.data ? 'approved' : 'rejected'}
          </div>)
        return null
      })}
      <div ref={end} />
    </div>
  )
}