// 应用状态管理Hook
import { useState, useEffect, createContext, useContext } from 'react';

// 应用状态上下文
const AppStateContext = createContext();

// 应用状态提供者组件
export const AppStateProvider = ({ children }) => {
    const [activeTab, setActiveTab] = useState('overview');
    const [connectionStatus, setConnectionStatus] = useState('disconnected');
    const [notifications, setNotifications] = useState([]);
    const [sseConnection, setSseConnection] = useState(null);

    // 切换标签页
    const switchTab = (tabName) => {
        setActiveTab(tabName);
        console.log(`📋 [布局] 切换到标签页: ${tabName}`);
    };

    // 添加通知
    const addNotification = (notification) => {
        const id = Date.now();
        const newNotification = { ...notification, id };
        setNotifications(prev => [...prev, newNotification]);

        // 5秒后自动移除通知
        setTimeout(() => {
            removeNotification(id);
        }, 5000);
    };

    // 移除通知
    const removeNotification = (id) => {
        setNotifications(prev => prev.filter(n => n.id !== id));
    };

    // 更新连接状态
    const updateConnectionStatus = (status) => {
        if (status !== connectionStatus) {
            setConnectionStatus(status);
            console.log(`📡 [连接状态] ${status}`);
        }
    };

    // SSE连接管理
    useEffect(() => {
        // 这里可以初始化SSE连接
        console.log('🚀 [应用状态] 初始化应用状态管理');

        return () => {
            if (sseConnection) {
                sseConnection.close();
            }
        };
    }, []);

    const value = {
        // 状态
        activeTab,
        connectionStatus,
        notifications,
        sseConnection,

        // 方法
        switchTab,
        addNotification,
        removeNotification,
        updateConnectionStatus,
        setSseConnection
    };

    return (
        <AppStateContext.Provider value={value}>
            {children}
        </AppStateContext.Provider>
    );
};

// 使用应用状态的Hook
export const useAppState = () => {
    const context = useContext(AppStateContext);
    if (!context) {
        throw new Error('useAppState must be used within an AppStateProvider');
    }
    return context;
};

// 导出默认值
export default useAppState;