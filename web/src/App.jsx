import { useEffect, useState } from 'react'
import Landing from './pages/Landing.jsx'
import Console from './pages/Console.jsx'

// tiny hash router: "#/" = home, "#/console" = the app
const route = () => (window.location.hash.startsWith('#/console') ? 'console' : 'home')

export default function App() {
  const [page, setPage] = useState(route())
  useEffect(() => {
    const on = () => { setPage(route()); if (route() === 'console') window.scrollTo(0, 0) }
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])
  return page === 'console' ? <Console /> : <Landing />
}