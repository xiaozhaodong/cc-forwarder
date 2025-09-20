// 配置项组件
// 渲染单个配置键值对，应用config-item和config-key CSS类

import React from 'react';
import ConfigValue from './ConfigValue.jsx';

const ConfigItem = ({ configKey, value }) => {
    return (
        <div className="config-item">
            <span className="config-key">{configKey}</span>
            <ConfigValue value={value} />
        </div>
    );
};

export default ConfigItem;