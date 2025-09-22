// 导航标签组件
import React from 'react';
import { useNavigation } from '../hooks/useNavigation.jsx';

const Navigation = () => {
    const { tabs, switchTab, isTabActive } = useNavigation();

    return (
        <nav className="nav-tabs">
            {tabs.map(tab => (
                <button
                    key={tab.name}
                    className={`nav-tab ${isTabActive(tab.name) ? 'active' : ''}`}
                    onClick={() => switchTab(tab.name)}
                >
                    {tab.label}
                </button>
            ))}
        </nav>
    );
};

export default Navigation;