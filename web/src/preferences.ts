type Preferences = { theme: 'system' | 'dark' | 'light'; active_store_id: string }

// The host's workspace owns preferences, including in external browsers. Native
// webviews can stay ephemeral without sharing localStorage between app copies.
let preferences: Preferences = { theme: 'system', active_store_id: '' }
let pending = Promise.resolve()

export async function loadPreferences() {
  const response = await fetch('/api/preferences', { cache: 'no-store' })
  if (!response.ok) throw new Error('无法读取界面配置')
  preferences = await response.json()
}

export function getPreferences(): Readonly<Preferences> { return preferences }

export function savePreferences(patch: Partial<Preferences>) {
  preferences = { ...preferences, ...patch }
  pending = pending.then(async () => {
    const response = await fetch('/api/preferences', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(patch),
    })
    if (!response.ok) throw new Error('无法保存界面配置')
  }).catch(error => { console.warn('界面配置保存失败', error) })
}
