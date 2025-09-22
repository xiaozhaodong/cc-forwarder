// 主内容区域组件
import React, { useState, useEffect, Suspense } from 'react';
import { useNavigation } from '../hooks/useNavigation.jsx';
import OverviewPage from '../../pages/overview/index.jsx';

// 动态组件加载器Hook
const useDynamicComponent = (tabName, componentPath) => {
    const [Component, setComponent] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!componentPath) return;

        let cancelled = false;
        setLoading(true);
        setError(null);

        const loadComponent = async () => {
            try {
                console.log(`📦 [动态组件] 加载: ${tabName} -> ${componentPath}`);

                const ComponentModule = await window.importReactModule(`pages/${componentPath}`);
                const LoadedComponent = ComponentModule.default || ComponentModule;

                if (!LoadedComponent) {
                    throw new Error(`组件模块加载失败: ${componentPath}`);
                }

                if (!cancelled) {
                    setComponent(() => LoadedComponent);
                    console.log(`✅ [动态组件] 加载成功: ${tabName}`);
                }
            } catch (err) {
                if (!cancelled) {
                    setError(err.message);
                    console.error(`❌ [动态组件] 加载失败: ${tabName}`, err);
                }
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        };

        loadComponent();

        return () => {
            cancelled = true;
        };
    }, [tabName, componentPath]);

    return { Component, loading, error };
};

// 单个标签页组件
const TabContent = ({ tabName, isActive }) => {
    const { getTabComponent } = useNavigation();

    // 概览页使用静态导入，避免重复编译
    if (tabName === 'overview') {
        return (
            <div
                id={tabName}
                className={`tab-content ${isActive ? 'active' : ''}`}
                style={{ display: isActive ? 'block' : 'none' }}
            >
                <div id={`react-${tabName}-container`}>
                    {!isActive && (
                        <div style={{ textAlign: 'center', padding: '48px 24px', color: '#6b7280' }}>
                            <div style={{ fontSize: '24px', marginBottom: '8px' }}>💤</div>
                            <p>标签页未激活</p>
                        </div>
                    )}

                    {isActive && (
                        <Suspense fallback={
                            <div style={{ textAlign: 'center', padding: '48px 24px', color: '#6b7280' }}>
                                <div style={{ fontSize: '24px', marginBottom: '8px' }}>⚡</div>
                                <p>组件渲染中...</p>
                            </div>
                        }>
                            <OverviewPage />
                        </Suspense>
                    )}
                </div>
            </div>
        );
    }

    // 其他页面使用动态导入
    const componentPath = getTabComponent(tabName);
    const { Component, loading, error } = useDynamicComponent(tabName, componentPath);

    return (
        <div
            id={tabName}
            className={`tab-content ${isActive ? 'active' : ''}`}
            style={{ display: isActive ? 'block' : 'none' }}
        >
            <div id={`react-${tabName}-container`}>
                {!isActive && (
                    <div style={{ textAlign: 'center', padding: '48px 24px', color: '#6b7280' }}>
                        <div style={{ fontSize: '24px', marginBottom: '8px' }}>💤</div>
                        <p>标签页未激活</p>
                    </div>
                )}

                {isActive && loading && (
                    <div style={{ textAlign: 'center', padding: '48px 24px', color: '#6b7280' }}>
                        <div style={{ fontSize: '24px', marginBottom: '8px' }}>⏳</div>
                        <p>React{tabName}页面加载中...</p>
                    </div>
                )}

                {isActive && error && (
                    <div style={{ textAlign: 'center', padding: '48px 24px', color: '#ef4444' }}>
                        <div style={{ fontSize: '48px', marginBottom: '16px' }}>❌</div>
                        <h3 style={{ margin: '0 0 8px 0' }}>模块加载失败</h3>
                        <p style={{ margin: '0', fontSize: '14px' }}>{error}</p>
                    </div>
                )}

                {isActive && Component && !loading && !error && (
                    <Suspense fallback={
                        <div style={{ textAlign: 'center', padding: '48px 24px', color: '#6b7280' }}>
                            <div style={{ fontSize: '24px', marginBottom: '8px' }}>⚡</div>
                            <p>组件渲染中...</p>
                        </div>
                    }>
                        <Component />
                    </Suspense>
                )}
            </div>
        </div>
    );
};

const MainContent = () => {
    const { activeTab } = useNavigation();

    const tabs = ['overview', 'charts', 'endpoints', 'groups', 'requests', 'config'];

    return (
        <main>
            {tabs.map(tabName => (
                <TabContent
                    key={tabName}
                    tabName={tabName}
                    isActive={activeTab === tabName}
                />
            ))}
        </main>
    );
};

export default MainContent;