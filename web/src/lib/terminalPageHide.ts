import { api } from '@/lib/api'

// Reloading or closing the page tears it down without running any component
// hook, so the close request a terminal sends when it unmounts never leaves.
// The Panel keeps the shell until it has been idle for 35 minutes, and a user
// owns only four, so a few reloads would use them all up. `pagehide` is the one
// event that fires for a reload, and a keepalive request is allowed to finish
// after the page is gone.
//
// A page entering the back/forward cache (`persisted`) keeps its state and may
// come back alive, so its terminals stay open.
export function closeTerminalsOnPageHide(sessions: () => Iterable<string>): () => void {
  const onPageHide = (event: PageTransitionEvent): void => {
    if (event.persisted) return
    for (const sessionId of sessions()) api.terminals.closeOnUnload(sessionId)
  }
  window.addEventListener('pagehide', onPageHide)
  return () => window.removeEventListener('pagehide', onPageHide)
}
