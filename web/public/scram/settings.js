// Pure normalize/defaults logic, kept free of localStorage I/O so it's
// testable under plain `node --test` without a DOM/localStorage shim.
export const SETTINGS_STORAGE_KEY = 'sshBrowserSettings';

export function normalizeSettings(raw) {
  const s = raw && typeof raw === 'object' ? raw : {};
  return {
    adblockEnabled: s.adblockEnabled === true,
  };
}

export function loadSettings(storage) {
  try {
    return normalizeSettings(JSON.parse(storage.getItem(SETTINGS_STORAGE_KEY) || '{}'));
  } catch (_) {
    return normalizeSettings({});
  }
}

export function saveSettings(storage, settings) {
  storage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(normalizeSettings(settings)));
}
