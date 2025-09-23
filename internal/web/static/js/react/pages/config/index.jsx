// 配置页面主组件
// 实现与原版HTML结构完全一致的配置页面

import React from 'react';
import useConfigData from './hooks/useConfigData.jsx';
import ConfigSection from './components/ConfigSection.jsx';
import { formatConfigData } from './utils/configFormatter.jsx';

const ConfigPage = () => {
    const { configData, loading, error, refetch } = useConfigData();

    // 加载状态
    if (loading) {
        return (
            <div className="section">
                <h2>⚙️ 当前配置</h2>
                <div id="config-display">
                    <p>加载中...</p>
                </div>
            </div>
        );
    }

    // 错误状态
    if (error) {
        return (
            <div className="section">
                <h2>⚙️ 当前配置</h2>
                <div id="config-display">
                    <div style={{
                        textAlign: 'center',
                        padding: '24px',
                        color: '#ef4444'
                    }}>
                        <div style={{ fontSize: '24px', marginBottom: '8px' }}>
                            ❌
                        </div>
                        <p style={{ margin: '0 0 16px 0' }}>
                            配置数据加载失败: {error}
                        </p>
                        <button
                            onClick={refetch}
                            style={{
                                padding: '8px 16px',
                                backgroundColor: '#ef4444',
                                color: 'white',
                                border: 'none',
                                borderRadius: '4px',
                                cursor: 'pointer'
                            }}
                        >
                            重试
                        </button>
                    </div>
                </div>
            </div>
        );
    }

    // 格式化配置数据
    const formattedSections = formatConfigData(configData);

    // 主要内容渲染 - 与原版HTML结构完全一致
    return (
        <div className="section">
            <h2>⚙️ 当前配置</h2>
            <div id="config-display">
                {formattedSections.length === 0 ? (
                    <p>暂无配置数据</p>
                ) : (
                    formattedSections.map(({ name, data }) => (
                        <ConfigSection
                            key={name}
                            sectionName={name}
                            sectionData={data}
                        />
                    ))
                )}
            </div>
        </div>
    );
};

export default ConfigPage;