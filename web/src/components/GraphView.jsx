import { useEffect, useRef, useState } from 'react'
import * as d3 from 'd3'

const KIND = { User: '#38bdf8', Group: '#a78bfa', Role: '#fbbf24', Policy: '#64748b', Resource: '#34d399' }
const keyOf = (from, to, type) => `${from}|${to}|${type}`

// d3 force graph. Attack paths animate in red; edges removed by the selected fix turn into grey dashes.
export default function GraphView({ graph, source, selIds, hiPath }) {
  const wrap = useRef(null)
  const svgRef = useRef(null)
  const pos = useRef({})
  const [size, setSize] = useState({ w: 800, h: 600 })

  useEffect(() => {
    const el = wrap.current
    const ro = new ResizeObserver(() => {
      const r = el.getBoundingClientRect()
      setSize({ w: Math.max(300, r.width), h: Math.max(300, r.height) })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  useEffect(() => {
    if (!graph) return
    const { w, h } = size
    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()
    svg.attr('viewBox', `0 0 ${w} ${h}`)

    const rm = new Set(selIds || [])
    const pathKeys = new Set()
    const blocked = new Set()
    graph.paths.forEach((p, pi) => {
      const killed = p.permission_ids.some((id) => rm.has(id))
      for (let i = 0; i < p.edge_types.length; i++) {
        const k = keyOf(p.nodes[i], p.nodes[i + 1], p.edge_types[i])
        if (hiPath != null && hiPath !== pi) continue
        pathKeys.add(k)
        if (killed) blocked.add(k)
      }
    })
    // when a path is highlighted, paths that are not highlighted stay dim; otherwise all paths glow
    const cls = (e) => {
      if (rm.has(e.id)) return 'removed'
      const k = keyOf(e.from, e.to, e.type)
      if (pathKeys.has(k) && !blocked.has(k)) return 'attack'
      return 'normal'
    }

    const defs = svg.append('defs')
    ;[['n', '#4b5b7c'], ['a', '#fb7185'], ['x', '#7c8aa5']].forEach(([id, c]) => {
      defs.append('marker').attr('id', `arr-${id}`).attr('viewBox', '0 -5 10 10').attr('refX', 24).attr('refY', 0)
        .attr('markerWidth', 6).attr('markerHeight', 6).attr('orient', 'auto')
        .append('path').attr('d', 'M0,-4L10,0L0,4').attr('fill', c)
    })
    const glow = defs.append('filter').attr('id', 'glow').attr('x', '-50%').attr('y', '-50%').attr('width', '200%').attr('height', '200%')
    glow.append('feGaussianBlur').attr('stdDeviation', 3).attr('result', 'b')
    const mg = glow.append('feMerge')
    mg.append('feMergeNode').attr('in', 'b'); mg.append('feMergeNode').attr('in', 'SourceGraphic')

    const root = svg.append('g')
    svg.call(d3.zoom().scaleExtent([0.4, 3]).on('zoom', (ev) => root.attr('transform', ev.transform)))

    const nodes = graph.nodes.map((n) => ({ ...n, ...(pos.current[n.id] || {}) }))
    const ids = new Set(nodes.map((n) => n.id))
    const links = graph.edges
      .filter((e) => ids.has(e.from) && ids.has(e.to))
      .map((e) => ({ ...e, source: e.from, target: e.to }))

    const link = root.append('g').selectAll('line').data(links).join('line')
      .attr('class', (l) => (cls(l) === 'attack' ? 'flow' : null))
      .attr('stroke', (l) => ({ attack: '#fb7185', removed: '#7c8aa5', normal: l.type === 'ESCALATES' ? '#b9622f' : '#2f3d5a' }[cls(l)]))
      .attr('stroke-width', (l) => (cls(l) === 'attack' ? 2.8 : 1.3))
      .attr('stroke-dasharray', (l) => {
        const c = cls(l)
        if (c === 'attack') return null // css animation owns the dash
        if (c === 'removed') return '5 4'
        if (l.type === 'ESCALATES') return '6 3'
        return l.type.startsWith('ALLOWS_ON') ? '2 3' : null
      })
      .attr('marker-end', (l) => `url(#arr-${cls(l) === 'attack' ? 'a' : cls(l) === 'removed' ? 'x' : 'n'})`)
      .attr('opacity', (l) => (hiPath != null && cls(l) === 'normal' ? 0.35 : 0.95))
    link.append('title').text((l) => `${l.type}${l.action ? ' ' + l.action : ''}  [${l.id}]${l.via ? '  via ' + l.via.join(',') : ''}`)

    const node = root.append('g').selectAll('g').data(nodes).join('g')
      .call(d3.drag()
        .on('start', (ev, d) => { if (!ev.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y })
        .on('drag', (ev, d) => { d.fx = ev.x; d.fy = ev.y })
        .on('end', (ev, d) => { if (!ev.active) sim.alphaTarget(0); d.fx = null; d.fy = null }))
    node.filter((d) => d.crown).append('circle').attr('class', 'ring').attr('r', 12).attr('fill', 'none').attr('stroke', '#fb7185').attr('stroke-width', 1.5)
    node.filter((d) => d.id === source).append('circle').attr('class', 'ring').attr('r', 10).attr('fill', 'none').attr('stroke', '#ffffff').attr('stroke-width', 1.5)
    node.append('circle')
      .attr('r', (d) => (d.crown ? 12 : d.kind === 'User' ? 10 : 8))
      .attr('fill', (d) => (d.crown ? '#fb7185' : KIND[d.kind] || '#999'))
      .attr('stroke', (d) => (d.id === source ? '#ffffff' : '#05070d'))
      .attr('stroke-width', (d) => (d.id === source ? 3 : 1.5))
      .attr('filter', (d) => (d.crown || d.id === source ? 'url(#glow)' : null))
    node.append('text').text((d) => d.id).attr('x', 14).attr('y', 4).attr('font-size', 11).attr('fill', '#cbd5e8')
      .attr('paint-order', 'stroke').attr('stroke', '#05070d').attr('stroke-width', 3.5)
    node.append('title').text((d) => `${d.kind}${d.crown ? ' · crown jewel' : ''}`)

    const sim = d3.forceSimulation(nodes)
      .force('link', d3.forceLink(links).id((d) => d.id).distance(78).strength(0.5))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(w / 2, h / 2))
      .force('collide', d3.forceCollide(22))
      .alpha(Object.keys(pos.current).length ? 0.08 : 1)
      .on('tick', () => {
        link.attr('x1', (d) => d.source.x).attr('y1', (d) => d.source.y).attr('x2', (d) => d.target.x).attr('y2', (d) => d.target.y)
        node.attr('transform', (d) => `translate(${d.x},${d.y})`)
        nodes.forEach((n) => { pos.current[n.id] = { x: n.x, y: n.y } })
      })
    return () => sim.stop()
  }, [graph, source, selIds, hiPath, size])

  return (
    <div ref={wrap} className="relative h-full w-full">
      <svg ref={svgRef} className="h-full w-full" />
      <div className="pointer-events-none absolute bottom-3 left-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-slate-400">
        {Object.entries(KIND).map(([k, c]) => (
          <span key={k} className="flex items-center gap-1.5"><i className="inline-block h-2.5 w-2.5 rounded-full" style={{ background: c }} />{k.toLowerCase()}</span>
        ))}
        <span className="flex items-center gap-1.5"><i className="inline-block h-2.5 w-2.5 rounded-full bg-rose-400" />crown jewel</span>
        <span className="text-rose-400">━ attack path</span>
        <span className="text-slate-400">╌ removed by fix</span>
      </div>
    </div>
  )
}