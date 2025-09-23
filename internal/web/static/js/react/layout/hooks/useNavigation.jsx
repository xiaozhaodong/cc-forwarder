// 导航管理Hook
import { useCallback } from 'react';
import { useAppState } from './useAppState.jsx';

// 标签页配置
const TAB_CONFIG = {
    overview: {
        name: 'overview',
        label: '📊 概览',
        component: 'overview/index.jsx'
    },
    // charts: {
    //     name: 'charts',
    //     label: '📈 图表',
    //     component: 'charts/index.jsx'
    // },
    endpoints: {
        name: 'endpoints',
        label: '📡 端点',
        component: 'endpoints/index.jsx'
    },
    groups: {
        name: 'groups',
        label: '📦 组管理',
        component: 'groups/index.jsx'
    },
    requests: {
        name: 'requests',
        label: '📊 请求追踪',
        component: 'requests/index.jsx'
    },
    config: {
        name: 'config',
        label: '⚙️ 配置',
        component: 'config/index.jsx'
    }
};

export const useNavigation = () => {
    const { activeTab, switchTab } = useAppState();

    // 获取所有标签页
    const getTabs = useCallback(() => {
        return Object.values(TAB_CONFIG);
    }, []);

    // 获取当前标签页配置
    const getCurrentTab = useCallback(() => {
        return TAB_CONFIG[activeTab] || TAB_CONFIG.overview;
    }, [activeTab]);

    // 切换标签页
    const handleTabSwitch = useCallback((tabName) => {
        if (TAB_CONFIG[tabName]) {
            switchTab(tabName);
        } else {
            console.warn(`⚠️ [导航] 未知的标签页: ${tabName}`);
        }
    }, [switchTab]);

    // 检查标签页是否激活
    const isTabActive = useCallback((tabName) => {
        return activeTab === tabName;
    }, [activeTab]);

    // 获取标签页组件路径
    const getTabComponent = useCallback((tabName) => {
        const tab = TAB_CONFIG[tabName];
        return tab ? tab.component : null;
    }, []);

    return {
        activeTab,
        tabs: getTabs(),
        currentTab: getCurrentTab(),
        switchTab: handleTabSwitch,
        isTabActive,
        getTabComponent
    };
};

export default useNavigation;