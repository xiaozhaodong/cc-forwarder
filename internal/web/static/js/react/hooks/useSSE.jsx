// SSE数据更新Hook - 集成后端实时推送
// 2025-09-15 16:55:17

import { useState, useRef, useCallback, useEffect } from 'react';

// 生成或获取客户端ID
const getOrCreateClientId = () => {
    let clientId = localStorage.getItem('client_id');
    if (!clientId) {
        clientId = 'client_' + Math.random().toString(36).substring(2, 11);
        localStorage.setItem('client_id', clientId);
    }
    return clientId;
};

// SSE连接Hook
const useSSE = (onDataUpdate) => {
    const [connectionStatus, setConnectionStatus] = useState('disconnected');
    const [reconnectAttempts, setReconnectAttempts] = useState(0);
    const connectionRef = useRef(null);
    const reconnectTimerRef = useRef(null);

    const MAX_RECONNECT_ATTEMPTS = 5;
    const RECONNECT_DELAY = 3000;

    const connect = useCallback(() => {
        if (connectionRef.current) {
            return; // 已有连接
        }

        const clientId = getOrCreateClientId();
        const events = 'status,endpoint,group,connection,log,chart';

        try {
            console.log('🔄 [SSE React] 建立SSE连接...');
            connectionRef.current = new EventSource(`/api/v1/stream?client_id=${clientId}&events=${events}`);

            connectionRef.current.onopen = () => {
                console.log('📡 [SSE React] SSE连接已建立');
                setConnectionStatus('connected');
                setReconnectAttempts(0);
            };

            connectionRef.current.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    console.log('🔍 [useSSE调试] onmessage收到数据:', data);
                    if (onDataUpdate) {
                        onDataUpdate(data);
                    }
                } catch (error) {
                    console.error('❌ [SSE React] 解析SSE消息失败:', error, event.data);
                }
            };

            // 监听特定事件类型
            ['status', 'endpoint', 'group', 'connection', 'log', 'chart'].forEach(eventType => {
                connectionRef.current.addEventListener(eventType, (event) => {
                    try {
                        const data = JSON.parse(event.data);
                        console.log(`🔍 [useSSE调试] addEventListener收到${eventType}事件:`, data);
                        if (onDataUpdate) {
                            onDataUpdate(data, eventType);
                        }
                    } catch (error) {
                        console.error(`❌ [SSE React] 解析${eventType}事件失败:`, error);
                    }
                });
            });

            connectionRef.current.onerror = (event) => {
                console.error('❌ [SSE React] SSE连接错误:', event);
                setConnectionStatus('error');
                handleReconnect();
            };

        } catch (error) {
            console.error('❌ [SSE React] 创建SSE连接失败:', error);
            setConnectionStatus('error');
            handleReconnect();
        }
    }, [onDataUpdate]);

    const handleReconnect = useCallback(() => {
        if (connectionRef.current) {
            connectionRef.current.close();
            connectionRef.current = null;
        }

        setReconnectAttempts(prev => {
            const newAttempts = prev + 1;

            if (newAttempts <= MAX_RECONNECT_ATTEMPTS) {
                console.log(`🔄 [SSE React] 准备重连，尝试次数: ${newAttempts}/${MAX_RECONNECT_ATTEMPTS}`);
                setConnectionStatus('reconnecting');

                reconnectTimerRef.current = setTimeout(() => {
                    connect();
                }, RECONNECT_DELAY);
            } else {
                console.error('❌ [SSE React] 重连次数已达上限，停止重连');
                setConnectionStatus('failed');
            }

            return newAttempts;
        });
    }, [connect]);

    const disconnect = useCallback(() => {
        if (reconnectTimerRef.current) {
            clearTimeout(reconnectTimerRef.current);
            reconnectTimerRef.current = null;
        }

        if (connectionRef.current) {
            connectionRef.current.close();
            connectionRef.current = null;
        }

        setConnectionStatus('disconnected');
        console.log('📡 [SSE React] SSE连接已断开');
    }, []);

    useEffect(() => {
        connect();

        return () => {
            disconnect();
        };
    }, [connect, disconnect]);

    return {
        connectionStatus,
        reconnectAttempts,
        connect,
        disconnect
    };
};

export default useSSE;