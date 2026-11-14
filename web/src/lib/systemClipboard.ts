// Snapshot File objects during the native paste event: clipboardData may be
// cleared by the browser as soon as the event handler returns.
export function clipboardFiles(event: ClipboardEvent): File[] {
  const data = event.clipboardData
  if (!data) return []
  const files = data.files?.length ? Array.from(data.files) : Array.from(data.items || [])
    .filter((item) => item.kind === 'file')
    .flatMap((item) => { const file = item.getAsFile(); return file ? [file] : [] })
  const stamp = Date.now()
  return files.map((file, index) => {
    if (!file.type.startsWith('image/') || (file.name && !/^image\.(png|jpe?g|gif|webp|bmp)$/i.test(file.name))) return file
    const extension = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/gif': 'gif', 'image/webp': 'webp', 'image/bmp': 'bmp' }[file.type]
    return extension ? new File([file], `clipboard-${stamp}-${index + 1}.${extension}`, { type: file.type, lastModified: file.lastModified }) : file
  })
}

export function keepsNativeClipboard(target: EventTarget | null): boolean {
  return target instanceof Element && Boolean(target.closest(
    'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"], .cm-editor, .xterm, [role="dialog"], [aria-modal="true"]',
  ))
}
