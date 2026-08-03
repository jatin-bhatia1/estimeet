import { useCallback, useEffect, useRef, useState } from 'react'

import type { RoomState } from './types'

export type ConnectionStatus = 'connecting' | 'open' | 'reconnecting' | 'offline'

export interface RoomEvent {
  type: string
  payload?: unknown
  at: number
}

interface SocketMessage {
  type: string
  payload?: unknown
}

const MAX_BACKOFF_MS = 15_000
const KEEPALIVE_MS = 30_000

/**
 * useRoomSocket keeps a live room snapshot. The server pushes a fresh,
 * per-viewer state after every change, so the UI never polls.
 */
export function useRoomSocket(code: string, token: string | null) {
  const [state, setState] = useState<RoomState | null>(null)
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [lastEvent, setLastEvent] = useState<RoomEvent | null>(null)
  const socketRef = useRef<WebSocket | null>(null)

  /** applyState lets REST mutations update the UI instantly, before the broadcast lands. */
  const applyState = useCallback((next: RoomState) => setState(next), [])

  useEffect(() => {
    if (!token) return

    let disposed = false
    let attempt = 0
    let reconnectTimer: number | undefined
    let keepAliveTimer: number | undefined

    const connect = () => {
      if (disposed) return
      setStatus(attempt === 0 ? 'connecting' : 'reconnecting')

      const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
      const url = `${scheme}://${window.location.host}/api/rooms/${encodeURIComponent(code)}/ws`
      // The bearer token travels as a subprotocol: a browser cannot set headers
      // on a WebSocket handshake, and a query string would leak into access logs.
      const socket = new WebSocket(url, ['estimeet.v1', `bearer.${token}`])
      socketRef.current = socket

      socket.onopen = () => {
        attempt = 0
        setStatus('open')
        keepAliveTimer = window.setInterval(() => {
          if (socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'ping' }))
          }
        }, KEEPALIVE_MS)
      }

      socket.onmessage = (event) => {
        let message: SocketMessage
        try {
          message = JSON.parse(event.data as string) as SocketMessage
        } catch {
          return
        }
        if (message.type === 'state') {
          setState(message.payload as RoomState)
          return
        }
        setLastEvent({ type: message.type, payload: message.payload, at: Date.now() })
      }

      socket.onerror = () => socket.close()

      socket.onclose = () => {
        window.clearInterval(keepAliveTimer)
        if (disposed) return
        attempt += 1
        setStatus(attempt > 4 ? 'offline' : 'reconnecting')
        const delay = Math.min(500 * 2 ** attempt, MAX_BACKOFF_MS)
        reconnectTimer = window.setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      disposed = true
      window.clearTimeout(reconnectTimer)
      window.clearInterval(keepAliveTimer)
      socketRef.current?.close()
      socketRef.current = null
    }
  }, [code, token])

  return { state, status, lastEvent, applyState }
}
