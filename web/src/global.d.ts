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
  interface RFBOptions {
    credentials?: { password?: string }
    wsProtocols?: string[]
  }

  export default class RFB {
    constructor(target: HTMLElement, url: string, options?: RFBOptions)
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

declare module 'guacamole-common-js' {
  // guacamole-common-js 1.5.0 无类型声明（UMD/ESM bundle，仅 export default）。
  // 按 @novnc/novnc 同款做法：只声明本项目用到的 API，不自由发挥协议细节。
  // 设计见 docs/remote-desktop-guacamole-design.md（D1/D2/D3）。

  export interface Tunnel {
    connect(data?: string): void
    disconnect(): void
    sendMessage(...elements: unknown[]): void
    oninstruction?: (opcode: string, parameters: string[]) => void
    onerror?: (status: { code: number; message: string }) => void
    onstatechange?: (state: number) => void
  }

  export interface DisplayElement {
    getElement(): HTMLElement
  }

  export interface MouseState {
    x: number
    y: number
    button: number
    buttons: number
  }

  const Guacamole: {
    /** WS 隧道：connect(data) 落到 URL query `?`+data，subprotocol 硬编码 "guacamole" */
    WebSocketTunnel: new (tunnelURL: string) => Tunnel
    Client: new (tunnel: Tunnel) => {
      connect(data?: string): void
      disconnect(): void
      getDisplay(): DisplayElement
      sendMouseState(state: MouseState): void
      sendKeyEvent(pressed: 0 | 1, keysym: number): void
      sendSize(width: number, height: number): void
      /**
       * 建远端剪贴板写入流（内部发 `clipboard` 指令）——剪贴板反向
       * （浏览器 → 远端）用，返回流配 StringWriter 写文本
       */
      createClipboardStream(mimetype: string): unknown
      onclipboard?: (
        streams: Record<string, unknown>,
        mimetype: string,
      ) => void
      onstatechange?: (state: number) => void
      onerror?: (status: { code: number; message: string }) => void
    }
    Mouse: new (element: HTMLElement) => {
      onmousedown?: (state: MouseState) => void
      onmouseup?: (state: MouseState) => void
      onmousemove?: (state: MouseState) => void
    }
    Keyboard: new (element: HTMLElement | Document) => {
      onkeydown?: (keysym: number) => void
      onkeyup?: (keysym: number) => void
    }
    StringReader: new (stream: unknown) => {
      ondata?: (chunk: string) => void
      onend?: () => void
    }
    /** 往输出流写文本（配 Client.createClipboardStream 的返回值） */
    StringWriter: new (stream: unknown) => {
      send(data: string): void
      onack?: (status: { code: number; message: string }) => void
    }
    /** Web Audio API AudioContext 单例工厂（RawAudioPlayer 内部播放用） */
    AudioContextFactory: {
      getAudioContext(): {
        suspend(): Promise<void>
        resume(): Promise<void>
        state: string
      } | null
    }
    /** .guac 会话录制回放：内部建 PlaybackTunnel + Client、自动 keyframe 帧索引 */
    SessionRecording: new (source: Blob) => {
      play(): void
      pause(): void
      seek(position: number, callback?: () => void): void
      getDisplay(): DisplayElement
      getDuration(): number
      getPosition(): number
      isPlaying(): boolean
      onplay?: () => void
      onpause?: () => void
      onseek?: (position: number) => void
      onerror?: (status: { code: number; message: string }) => void
    }
  }

  export default Guacamole
}

