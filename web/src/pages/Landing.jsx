import { ArrowRight, GitBranch, Scissors, FlaskConical, ShieldCheck, Bot, Waypoints, Zap, Lock, Network, KeyRound, FileCode } from 'lucide-react'
import HeroGraph from '../components/HeroGraph.jsx'

const FEATURES = [
  { icon: GitBranch, title: 'Multi-hop attack paths', body: 'Variable-length graph traversal from any identity to your crown jewels — user → group → role → policy → resource, hop by hop.' },
  { icon: Scissors, title: 'Minimum cut, not a flat list', body: 'The smallest set of permissions whose removal closes every path. Escalations are an AND of two permissions, so we search exactly instead of guessing with max-flow.' },
  { icon: FlaskConical, title: 'Counterfactual simulation', body: 'Every fix is simulated on the graph first: paths killed, crown jewels still reachable, permissions legit users would lose.' },
  { icon: ShieldCheck, title: 'Legit-access guard', body: 'Fixes that would break labeled legit access are rejected automatically. The fear of "what if it breaks prod" is the reason teams never fix anything.' },
  { icon: Bot, title: 'An agent that picks tools', body: 'Claude chooses among MCP tools — enumerate, min-cut, simulate, apply — and explains every decision from the paths it actually found.' },
  { icon: Lock, title: 'Human in the loop, verified result', body: 'Applying a fix needs approval. Afterwards the same attack-path query runs again from the database, so the "before → after" number is proof, not a claim.' },
]

const STEPS = [
  { n: '01', t: 'Map', d: 'IAM users, groups, roles and policies become nodes and edges in FalkorDB. Privilege escalation patterns become derived edges that remember which permissions they depend on.' },
  { n: '02', t: 'Find', d: 'Multi-hop Cypher traversal returns every path from an identity to a crown jewel, with the exact permission edges each hop relies on.' },
  { n: '03', t: 'Cut', d: 'The agent computes the smallest permission set that blocks all paths and ranks the options by how many legit permissions they cost.' },
  { n: '04', t: 'Prove', d: 'It simulates, asks for approval, applies, writes the IAM commands to a file and re-queries the graph to show the paths are gone.' },
]

const CHAIN = ['intern', 'developers', 'ec2-admin-role', 'EC2AdminPolicy', 'prod-db']
const LIMITS = [
  'Models explicit allow statements only — no conditions, SCPs or permission boundaries.',
  'Two privilege-escalation patterns are implemented: PassRole + RunInstances, and CreatePolicyVersion on an attached policy.',
  '“Legit access” is a labeled set in the demo data. A real deployment would derive it from usage logs.',
  'The cut is an exact search, built for graphs with tens of candidate permissions, not thousands.',
  'Remediation is generated as reviewable commands. Nothing touches a real AWS account.',
]

function Nav() {
  return (
    <nav className="fixed inset-x-0 top-0 z-30 border-b border-white/5 bg-[#05070d]/70 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center gap-6 px-5 py-3.5">
        <a href="#/" className="flex items-center gap-2 text-lg font-bold tracking-tight"><span>⛓</span> Chokepoint</a>
        <div className="hidden flex-1 items-center gap-6 text-sm text-slate-400 md:flex">
          <a href="#how" className="hover:text-white">How it works</a>
          <a href="#features" className="hover:text-white">Features</a>
          <a href="#demo" className="hover:text-white">Example</a>
          <a href="#limits" className="hover:text-white">Limits</a>
        </div>
        <span className="flex-1 md:hidden" />
        <a href="#/console" className="flex items-center gap-2 rounded-lg bg-white px-4 py-2 text-sm font-semibold text-slate-950 hover:bg-slate-200">Launch console <ArrowRight size={14} /></a>
      </div>
    </nav>
  )
}

function Hero() {
  return (
    <header className="relative isolate overflow-hidden pt-32 pb-24">
      <div className="absolute inset-0 -z-10"><HeroGraph /></div>
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(60%_50%_at_50%_0%,rgba(34,211,238,0.14),transparent_70%)]" />
      <div className="absolute inset-x-0 bottom-0 -z-10 h-40 bg-gradient-to-t from-[#05070d] to-transparent" />
      <div className="mx-auto max-w-5xl px-5 text-center">
        <span className="mb-6 inline-flex items-center gap-2 rounded-full border border-cyan-400/30 bg-cyan-400/10 px-4 py-1.5 text-xs font-medium text-cyan-300">
          <Zap size={13} /> Graph Hacks · Track 01 · Agents That Act on Connected Data · FalkorDB
        </span>
        <h1 className="text-5xl font-extrabold leading-[1.05] tracking-tight sm:text-7xl">
          Find the path.<br /><span className="shine-text">Cut the path.</span> Prove it.
        </h1>
        <p className="mx-auto mt-6 max-w-2xl text-lg text-slate-300">
          Security teams get a flat list of alerts. Chokepoint gives them the attack path — and an agent that finds the <b className="text-white">smallest set of permissions</b> to remove, checks that legit access survives, and verifies the paths are gone.
        </p>
        <div className="mt-9 flex flex-wrap items-center justify-center gap-3">
          <a href="#/console" className="flex items-center gap-2 rounded-xl bg-gradient-to-r from-cyan-400 to-violet-500 px-6 py-3 font-semibold text-slate-950 shadow-lg shadow-cyan-500/20 hover:brightness-110">
            Launch console <ArrowRight size={16} />
          </a>
          <a href="#how" className="rounded-xl border border-slate-600 px-6 py-3 font-medium text-slate-200 hover:border-slate-400">How it works</a>
        </div>
        <div className="mx-auto mt-14 grid max-w-3xl grid-cols-2 gap-3 sm:grid-cols-4">
          {[['Multi-hop', 'graph traversal'], ['Min-cut', 'as an agent tool'], ['MCP', 'tool selection'], ['Verified', 'from the graph']].map(([a, b]) => (
            <div key={a} className="glass rounded-xl px-4 py-3"><div className="font-bold text-white">{a}</div><div className="text-xs text-slate-400">{b}</div></div>
          ))}
        </div>
      </div>
    </header>
  )
}

function Problem() {
  return (
    <section className="mx-auto max-w-5xl px-5 py-20">
      <div className="grid items-center gap-10 md:grid-cols-2">
        <div>
          <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">Nobody sees how an intern reaches prod.</h2>
          <p className="mt-4 text-slate-300">
            Cloud IAM is a web of groups, roles, policies and trust relationships. Every permission looks harmless alone. Chained together, a junior account can launch a machine with an admin role, or quietly rewrite its own policy.
          </p>
          <p className="mt-3 text-slate-400">
            Teams don't fix these because the fix feels risky: <i>what if it breaks something?</i> Chokepoint answers that question before touching anything.
          </p>
        </div>
        <div className="glass rounded-2xl p-6 font-mono text-[13px] leading-relaxed">
          <div className="mb-3 text-xs uppercase tracking-widest text-slate-500">Flat alert list</div>
          <div className="space-y-1 text-slate-500"><div>⚠ iam:PassRole granted to group</div><div>⚠ ec2:RunInstances granted to group</div><div>⚠ iam:CreatePolicyVersion on DevPolicy</div><div className="text-slate-600">… 3 more findings, no context</div></div>
          <div className="my-4 border-t border-dashed border-slate-700" />
          <div className="mb-3 text-xs uppercase tracking-widest text-cyan-400">Chokepoint</div>
          <div className="text-rose-300">intern → developers → ec2-admin-role → prod-db</div>
          <div className="mt-2 text-emerald-300">fix: remove intern from developers</div>
        </div>
      </div>
    </section>
  )
}

function How() {
  return (
    <section id="how" className="mx-auto max-w-6xl scroll-mt-20 px-5 py-20">
      <h2 className="text-center text-3xl font-bold tracking-tight sm:text-4xl">How it works</h2>
      <p className="mx-auto mt-3 max-w-xl text-center text-slate-400">Four steps. Every one of them reads from, or writes to, the graph.</p>
      <div className="mt-12 grid gap-4 md:grid-cols-4">
        {STEPS.map((s) => (
          <div key={s.n} className="glass relative rounded-2xl p-6">
            <div className="mb-3 text-sm font-bold text-cyan-400">{s.n}</div>
            <div className="mb-2 text-xl font-bold">{s.t}</div>
            <p className="text-sm text-slate-400">{s.d}</p>
          </div>
        ))}
      </div>
    </section>
  )
}

function Features() {
  return (
    <section id="features" className="mx-auto max-w-6xl scroll-mt-20 px-5 py-20">
      <h2 className="text-center text-3xl font-bold tracking-tight sm:text-4xl">The graph decides. The agent acts.</h2>
      <div className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {FEATURES.map(({ icon: Icon, title, body }) => (
          <div key={title} className="glass group rounded-2xl p-6 transition hover:-translate-y-1 hover:border-cyan-400/40">
            <div className="mb-4 inline-flex rounded-xl bg-gradient-to-br from-cyan-400/20 to-violet-500/20 p-3 text-cyan-300"><Icon size={22} /></div>
            <div className="mb-2 text-lg font-semibold">{title}</div>
            <p className="text-sm leading-relaxed text-slate-400">{body}</p>
          </div>
        ))}
      </div>
    </section>
  )
}

function Example() {
  return (
    <section id="demo" className="mx-auto max-w-5xl scroll-mt-20 px-5 py-20">
      <h2 className="text-center text-3xl font-bold tracking-tight sm:text-4xl">One path, one cut</h2>
      <p className="mt-3 text-center text-slate-400">From the bundled demo dataset: an intern who ended up in the developers group.</p>
      <div className="glass mt-10 rounded-2xl p-6 sm:p-8">
        <div className="scroll-thin overflow-x-auto">
          <svg viewBox="0 0 900 150" className="mx-auto min-w-[760px]">
            <defs>
              <marker id="ah" viewBox="0 -5 10 10" refX="9" refY="0" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,-4L10,0L0,4" fill="#fb7185" /></marker>
            </defs>
            {CHAIN.map((name, i) => {
              const x = 70 + i * 190
              const last = i === CHAIN.length - 1
              return (
                <g key={name}>
                  {i < CHAIN.length - 1 && <line x1={x + 28} y1="60" x2={x + 190 - 30} y2="60" stroke="#fb7185" strokeWidth="2.5" className="flow" markerEnd="url(#ah)" />}
                  <circle cx={x} cy="60" r={last ? 22 : 17} fill={last ? '#fb7185' : i === 0 ? '#38bdf8' : '#475569'} stroke={i === 0 ? '#fff' : '#05070d'} strokeWidth="3" />
                  {last && <circle cx={x} cy="60" r="22" fill="none" stroke="#fb7185" strokeWidth="1.5" className="ring" />}
                  <text x={x} y="108" textAnchor="middle" fill="#cbd5e8" fontSize="13">{name}</text>
                </g>
              )
            })}
            <g>
              <line x1="165" y1="30" x2="195" y2="90" stroke="#34d399" strokeWidth="3" />
              <line x1="195" y1="30" x2="165" y2="90" stroke="#34d399" strokeWidth="3" />
              <text x="180" y="22" textAnchor="middle" fill="#34d399" fontSize="12" fontWeight="700">CUT</text>
            </g>
          </svg>
        </div>
        <div className="mt-6 grid gap-3 sm:grid-cols-3">
          <div className="rounded-xl bg-slate-950/60 p-4"><div className="text-xs uppercase tracking-wider text-slate-500">Finds</div><div className="mt-1 text-sm text-slate-300">Every path from <span className="font-mono text-slate-100">intern</span> to the three crown jewels.</div></div>
          <div className="rounded-xl bg-slate-950/60 p-4"><div className="text-xs uppercase tracking-wider text-slate-500">Compares</div><div className="mt-1 text-sm text-slate-300">A one-permission fix that kills everything vs. fixes that only kill part of it.</div></div>
          <div className="rounded-xl bg-slate-950/60 p-4"><div className="text-xs uppercase tracking-wider text-slate-500">Proves</div><div className="mt-1 text-sm text-slate-300">Re-runs the query after the change. The number on screen comes from the graph.</div></div>
        </div>
      </div>
    </section>
  )
}

function Stack() {
  const items = [[Network, 'FalkorDB', 'graph + Cypher'], [FileCode, 'Go', 'core, API, MCP server'], [KeyRound, 'MCP', 'tools the agent selects'], [Bot, 'Claude', 'tool-using agent'], [Waypoints, 'React + Tailwind', 'this console']]
  return (
    <section className="mx-auto max-w-5xl px-5 py-14">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {items.map(([Icon, a, b]) => (
          <div key={a} className="glass rounded-xl p-4 text-center"><Icon className="mx-auto mb-2 text-cyan-300" size={20} /><div className="text-sm font-semibold">{a}</div><div className="text-[11px] text-slate-500">{b}</div></div>
        ))}
      </div>
    </section>
  )
}

function Limits() {
  return (
    <section id="limits" className="mx-auto max-w-4xl scroll-mt-20 px-5 py-20">
      <h2 className="text-center text-3xl font-bold tracking-tight sm:text-4xl">What it does not do</h2>
      <p className="mt-3 text-center text-slate-400">A security tool that overclaims is worse than none. These are the real limits of this version.</p>
      <ul className="mt-8 space-y-3">
        {LIMITS.map((l) => (
          <li key={l} className="glass flex gap-3 rounded-xl p-4 text-sm text-slate-300"><span className="mt-0.5 text-amber-300">▲</span>{l}</li>
        ))}
      </ul>
    </section>
  )
}

function Cta() {
  return (
    <section className="px-5 pb-24">
      <div className="mx-auto max-w-4xl overflow-hidden rounded-3xl border border-cyan-400/20 bg-gradient-to-br from-cyan-500/10 via-violet-500/10 to-rose-500/10 p-10 text-center">
        <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">See the path disappear.</h2>
        <p className="mx-auto mt-3 max-w-lg text-slate-300">Open the console, run the agent, and watch the attack paths go from red to none.</p>
        <a href="#/console" className="mt-7 inline-flex items-center gap-2 rounded-xl bg-white px-7 py-3 font-semibold text-slate-950 hover:bg-slate-200">Launch console <ArrowRight size={16} /></a>
      </div>
    </section>
  )
}

export default function Landing() {
  return (
    <div className="min-h-full bg-[#05070d] text-slate-100">
      <Nav />
      <Hero />
      <Problem />
      <How />
      <Features />
      <Example />
      <Stack />
      <Limits />
      <Cta />
      <footer className="border-t border-white/5 py-8 text-center text-xs text-slate-500">
        Chokepoint · built for Graph Hacks by WeMakeDevs × FalkorDB
      </footer>
    </div>
  )
}