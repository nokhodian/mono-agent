// Wails v2 runtime bridge — auto-generated stubs.
// These are overwritten by `wails build`. Used during development.

export function Call(bindingName, ...args) {
  if (window.go) {
    const parts = bindingName.split('.')
    // "main.App.Method" → window.go.main.App.Method
    let obj = window.go
    for (const p of parts) {
      obj = obj[p]
      if (!obj) throw new Error(`Wails binding not found: ${bindingName}`)
    }
    return obj(...args)
  }
  return Promise.reject(new Error('Wails runtime not available'))
}

export function EventsOn(eventName, callback) {
  if (window.runtime) return window.runtime.EventsOn(eventName, callback)
}

export function EventsOff(...eventNames) {
  if (window.runtime) return window.runtime.EventsOff(...eventNames)
}

export function EventsEmit(eventName, ...data) {
  if (window.runtime) return window.runtime.EventsEmit(eventName, ...data)
}

export function LogDebug(msg) { if (window.runtime) window.runtime.LogDebug(msg) }
export function LogInfo(msg)  { if (window.runtime) window.runtime.LogInfo(msg) }
export function LogWarning(msg){ if (window.runtime) window.runtime.LogWarning(msg) }
export function LogError(msg) { if (window.runtime) window.runtime.LogError(msg) }

export function WindowMinimise()   { if (window.runtime) window.runtime.WindowMinimise() }
export function WindowMaximise()   { if (window.runtime) window.runtime.WindowMaximise() }
export function WindowUnmaximise() { if (window.runtime) window.runtime.WindowUnmaximise() }
export function WindowClose()      { if (window.runtime) window.runtime.Quit() }
