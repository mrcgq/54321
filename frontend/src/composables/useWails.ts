// Wails 运行时封装
import { ref, onMounted, onUnmounted } from 'vue'

// 定义 Wails 运行时类型
declare global {
  interface Window {
    runtime: {
      EventsOn(eventName: string, callback: (...args: any[]) => void): () => void
      EventsOff(eventName: string, ...additionalEventNames: string[]): void
      EventsEmit(eventName: string, ...optionalData: any[]): void
      WindowMinimise(): void
      WindowMaximise(): void
      WindowUnmaximise(): void
      WindowToggleMaximise(): void
      WindowFullscreen(): void
      WindowUnfullscreen(): void
      WindowSetSize(width: number, height: number): void
      WindowGetSize(): Promise<{ w: number; h: number }>
      WindowSetMinSize(width: number, height: number): void
      WindowSetMaxSize(width: number, height: number): void
      WindowCenter(): void
      WindowShow(): void
      WindowHide(): void
      WindowSetTitle(title: string): void
      Quit(): void
      Environment(): Promise<{ buildType: string; platform: string; arch: string }>
      BrowserOpenURL(url: string): void
      ClipboardGetText(): Promise<string>
      ClipboardSetText(text: string): Promise<void>
    }
    go: {
      main: {
        App: Record<string, (...args: any[]) => Promise<any>>
      }
    }
  }
}

// 检查是否在 Wails 环境中
export function isWailsEnvironment(): boolean {
  return typeof window !== 'undefined' && typeof window.runtime !== 'undefined'
}

// 事件监听 Hook
export function useWailsEvent<T = any>(eventName: string, callback: (data: T) => void) {
  let unsubscribe: (() => void) | null = null

  onMounted(() => {
    if (isWailsEnvironment()) {
      unsubscribe = window.runtime.EventsOn(eventName, callback)
    }
  })

  onUnmounted(() => {
    if (unsubscribe) {
      unsubscribe()
    }
  })
}

// 多事件监听 Hook
export function useWailsEvents(events: Record<string, (data: any) => void>) {
  const unsubscribers: Array<() => void> = []

  onMounted(() => {
    if (isWailsEnvironment()) {
      for (const [eventName, callback] of Object.entries(events)) {
        const unsub = window.runtime.EventsOn(eventName, callback)
        unsubscribers.push(unsub)
      }
    }
  })

  onUnmounted(() => {
    unsubscribers.forEach(unsub => unsub())
  })
}

// 窗口控制 Hook
export function useWindowControl() {
  function minimize() {
    if (isWailsEnvironment()) {
      window.runtime.WindowMinimise()
    }
  }

  function maximize() {
    if (isWailsEnvironment()) {
      window.runtime.WindowMaximise()
    }
  }

  function toggleMaximize() {
    if (isWailsEnvironment()) {
      window.runtime.WindowToggleMaximise()
    }
  }

  function hide() {
    if (isWailsEnvironment()) {
      window.runtime.WindowHide()
    }
  }

  function show() {
    if (isWailsEnvironment()) {
      window.runtime.WindowShow()
    }
  }

  function quit() {
    if (isWailsEnvironment()) {
      window.runtime.Quit()
    }
  }

  function setTitle(title: string) {
    if (isWailsEnvironment()) {
      window.runtime.WindowSetTitle(title)
    }
  }

  return {
    minimize,
    maximize,
    toggleMaximize,
    hide,
    show,
    quit,
    setTitle
  }
}

// 剪贴板 Hook
export function useClipboard() {
  const text = ref('')

  async function read(): Promise<string> {
    if (isWailsEnvironment()) {
      text.value = await window.runtime.ClipboardGetText()
      return text.value
    }
    return ''
  }

  async function write(content: string): Promise<void> {
    if (isWailsEnvironment()) {
      await window.runtime.ClipboardSetText(content)
      text.value = content
    }
  }

  return {
    text,
    read,
    write
  }
}

// 打开浏览器链接
export function openURL(url: string) {
  if (isWailsEnvironment()) {
    window.runtime.BrowserOpenURL(url)
  } else {
    window.open(url, '_blank')
  }
}

// 调用 Go 后端方法
export async function callBackend<T = any>(method: string, ...args: any[]): Promise<T> {
  if (!isWailsEnvironment()) {
    throw new Error('Not in Wails environment')
  }

  const fn = window.go.main.App[method]
  if (typeof fn !== 'function') {
    throw new Error(`Method ${method} not found`)
  }

  return fn(...args)
}

// 系统托盘相关（需要在 Go 端实现）
export function useTray() {
  function showNotification(title: string, message: string) {
    // 通过后端调用系统通知
    if (isWailsEnvironment()) {
      callBackend('ShowNotification', title, message).catch(console.error)
    }
  }

  return {
    showNotification
  }
}
