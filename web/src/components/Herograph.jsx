import { useEffect, useRef } from 'react'

// Decorative animated network behind the hero. A few "attack path" edges pulse red.
export default function HeroGraph() {
  const ref = useRef(null)

  useEffect(() => {
    const canvas = ref.current
    const ctx = canvas.getContext('2d')
    let raf, w = 0, h = 0
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const N = 46
    const nodes = []

    const resize = () => {
      const r = canvas.getBoundingClientRect()
      w = r.width; h = r.height
      canvas.width = w * dpr; canvas.height = h * dpr
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    }
    resize()
    for (let i = 0; i < N; i++) {
      nodes.push({
        x: Math.random() * w, y: Math.random() * h,
        vx: (Math.random() - 0.5) * 0.25, vy: (Math.random() - 0.5) * 0.25,
        r: 1.6 + Math.random() * 2.2, hot: i % 11 === 0,
      })
    }
    // a fixed little chain that always pulses: "intern -> ... -> crown jewel"
    const chain = [3, 12, 21, 30, 39]

    const tick = (t) => {
      ctx.clearRect(0, 0, w, h)
      for (const n of nodes) {
        n.x += n.vx; n.y += n.vy
        if (n.x < 0 || n.x > w) n.vx *= -1
        if (n.y < 0 || n.y > h) n.vy *= -1
      }
      for (let i = 0; i < N; i++) {
        for (let j = i + 1; j < N; j++) {
          const a = nodes[i], b = nodes[j]
          const d = Math.hypot(a.x - b.x, a.y - b.y)
          if (d < 150) {
            ctx.strokeStyle = `rgba(110,150,220,${0.16 * (1 - d / 150)})`
            ctx.lineWidth = 1
            ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y); ctx.stroke()
          }
        }
      }
      const pulse = 0.5 + 0.5 * Math.sin(t / 420)
      ctx.lineWidth = 2
      ctx.strokeStyle = `rgba(251,113,133,${0.35 + 0.5 * pulse})`
      ctx.shadowColor = 'rgba(251,113,133,0.9)'
      ctx.shadowBlur = 10
      ctx.beginPath()
      chain.forEach((idx, k) => { const n = nodes[idx]; k ? ctx.lineTo(n.x, n.y) : ctx.moveTo(n.x, n.y) })
      ctx.stroke()
      ctx.shadowBlur = 0
      for (const n of nodes) {
        ctx.fillStyle = n.hot ? '#fb7185' : 'rgba(150,185,255,0.8)'
        ctx.beginPath(); ctx.arc(n.x, n.y, n.r, 0, Math.PI * 2); ctx.fill()
      }
      raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    window.addEventListener('resize', resize)
    return () => { cancelAnimationFrame(raf); window.removeEventListener('resize', resize) }
  }, [])

  return <canvas ref={ref} className="absolute inset-0 h-full w-full" aria-hidden="true" />
}