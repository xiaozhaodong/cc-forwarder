// 主应用组件
import { useEffect } from 'react';
import { AppStateProvider, useAppState } from './hooks/useAppState.jsx';
import { useNavigation } from './hooks/useNavigation.jsx';
import Header from './components/Header.jsx';
import Navigation from './components/Navigation.jsx';
import MainContent from './components/MainContent.jsx';
import ErrorBoundary from './components/ErrorBoundary.jsx';

// 内部App组件，用于访问hooks
const InnerApp = () => {
    const { switchTab } = useNavigation();

    // 导出全局showTab函数
    useEffect(() => {
        window.showTab = switchTab;
        console.log('✅ [React App] showTab函数已导出到全局');

        return () => {
            delete window.showTab;
        };
    }, [switchTab]);

    return (
        <div className="container">
            <ErrorBoundary>
                <Header />
            </ErrorBoundary>
            <ErrorBoundary>
                <Navigation />
            </ErrorBoundary>
            <ErrorBoundary>
                <MainContent />
            </ErrorBoundary>
        </div>
    );
};

const App = () => {
    // 组件挂载时加载样式
    useEffect(() => {
        console.log('🚀 [React App] 应用启动');

        // 清理函数
        return () => {
            console.log('🔄 [React App] 应用卸载');
        };
    }, []);

    return (
        <ErrorBoundary>
            <AppStateProvider>
                <InnerApp />
            </AppStateProvider>
        </ErrorBoundary>
    );
};

export default App;