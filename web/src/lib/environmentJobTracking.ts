const storageKey = 'kpanel:active-environment-job'
const validJobID = /^[a-f0-9]{32}$/

export function readTrackedEnvironmentJob(): string | undefined {
  try {
    const id = window.localStorage.getItem(storageKey)
    return id && validJobID.test(id) ? id : undefined
  } catch {
    return undefined
  }
}

export function saveTrackedEnvironmentJob(id?: string): void {
  try {
    if (id && validJobID.test(id)) window.localStorage.setItem(storageKey, id)
    else window.localStorage.removeItem(storageKey)
  } catch {
    // The current view and server-side running-job discovery remain available.
  }
}
