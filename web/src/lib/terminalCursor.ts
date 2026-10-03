import type { ITerminalOptions } from '@xterm/xterm'

// One cursor for every terminal. A one-pixel bar is hard to find on a dark
// shell; a steady block is, and stays readable because the theme paints the
// character under it with the shell background (`cursorAccent`). It does not
// blink, so it never disappears while the user looks for it. When the terminal
// loses focus xterm draws the block as an outline, so the user can tell which
// terminal takes the keyboard. Programs that choose their own cursor shape
// (vim, readline modes) still override this through the terminal's own escape
// sequence.
export const terminalCursorOptions = {
  cursorBlink: false,
  cursorStyle: 'block',
  cursorInactiveStyle: 'outline',
} as const satisfies Pick<ITerminalOptions, 'cursorBlink' | 'cursorStyle' | 'cursorInactiveStyle'>
