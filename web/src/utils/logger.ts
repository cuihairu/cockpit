/**
 * 统一的日志工具
 * 生产环境只记录错误，开发环境可记录更多信息
 */

const isDev = import.meta.env.MODE === 'development'

type LogLevel = 'info' | 'warn' | 'error' | 'debug'

class Logger {
  private formatMessage(level: LogLevel, ...args: any[]): string {
    const timestamp = new Date().toISOString()
    const message = args.map(arg => {
      if (typeof arg === 'object') {
        try {
          return JSON.stringify(arg)
        } catch {
          return String(arg)
        }
      }
      return String(arg)
    }).join(' ')
    return `[${timestamp}] [${level.toUpperCase()}] ${message}`
  }

  info(...args: any[]): void {
    if (isDev) {
      console.info(...args)
    }
  }

  warn(...args: any[]): void {
    if (isDev) {
      console.warn(...args)
    }
  }

  error(...args: any[]): void {
    // 错误始终记录
    console.error(this.formatMessage('error', ...args))
  }

  debug(...args: any[]): void {
    if (isDev) {
      console.log(...args)
    }
  }
}

export const logger = new Logger()
