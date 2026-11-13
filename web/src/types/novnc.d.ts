declare module '@novnc/novnc' {
  export default class RFB extends EventTarget {
    constructor(target: HTMLElement, channel: string | WebSocket)
    scaleViewport: boolean
    viewOnly: boolean
    resizeSession: boolean
    disconnect(): void
    sendCtrlAltDel(): void
    sendCredentials(credentials: { password: string }): void
    clipboardPasteFrom(text: string): void
    sendKey(keysym: number, code: string, down?: boolean): void
  }
}
