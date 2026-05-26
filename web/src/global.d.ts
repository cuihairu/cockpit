declare module '*.svg' {
  const content: string
  export default content
}

declare module '*.css' {
  const content: { [className: string]: string }
  export default content
}

declare module '*.less' {
  const content: { [className: string]: string }
  export default content
}

declare module '@novnc/novnc' {
  export default class RFB {
    constructor(target: HTMLElement, url: string, options?: { credentials?: { password?: string } })
    disconnect(): void
    sendCredentials(creds: { password: string }): void
    sendKey(keysym: number, code: string, down?: boolean): void
    sendCtrlAltDel(): void
    focus(): void
    blur(): void
    scaleViewport: boolean
    resizeSession: boolean
    viewOnly: boolean
    addEventListener(event: string, handler: (e: Event) => void): void
    removeEventListener(event: string, handler: (e: Event) => void): void
  }
}
