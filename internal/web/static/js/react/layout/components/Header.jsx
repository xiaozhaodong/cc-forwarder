// 页面头部组件
import React from 'react';
import ConnectionIndicator from './ConnectionIndicator.jsx';

const Header = () => {
    return (
        <header>
            <h1>🌐 Claude Request Forwarder</h1>
            <p>高性能API请求转发器 - Web监控界面</p>
            <ConnectionIndicator />
        </header>
    );
};

export default Header;