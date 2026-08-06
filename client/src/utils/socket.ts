import { useEffect, useRef } from 'react'
import { io, Socket } from 'socket.io-client'

let socket: Socket | null = null

function getSocket(): Socket {
  if (!socket) {
    socket = io('/payment', {
      transports: ['websocket', 'polling'],
      autoConnect: false,
    })
  }
  return socket
}

/**
 * 订阅订单状态变更，返回实时推送的 status。
 * 自动管理连接/断开，组件卸载时清理。
 */
export function useOrderStatus(
  orderNo: string | null,
  onStatus: (status: string) => void,
) {
  const socketRef = useRef<Socket | null>(null)

  useEffect(() => {
    if (!orderNo) return

    const s = getSocket()
    socketRef.current = s

    if (!s.connected) s.connect()

    const handleStatus = (data: { orderNo: string; status: string }) => {
      if (data.orderNo === orderNo) {
        onStatus(data.status)
      }
    }

    s.emit('subscribeOrder', orderNo)
    s.on('paymentStatus', handleStatus)

    return () => {
      s.off('paymentStatus', handleStatus)
      // 所有组件都卸载后断开
      if (s.connected && s.listeners('paymentStatus').length === 0) {
        s.disconnect()
      }
    }
  }, [orderNo, onStatus])
}

/**
 * 手动断开 Socket.IO 连接（用于登出等场景）
 */
export function disconnectSocket() {
  if (socket?.connected) {
    socket.disconnect()
    socket = null
  }
}
