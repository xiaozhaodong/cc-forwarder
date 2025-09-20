// 配置区块组件
// 渲染配置区块，包含标题和配置项列表，应用config-section CSS类

import React from 'react';
import ConfigItem from './ConfigItem.jsx';
import { getConfigItems } from '../utils/configFormatter.jsx';

const ConfigSection = ({ sectionName, sectionData }) => {
    const items = getConfigItems(sectionData);

    return (
        <div className="config-section">
            <h3>{sectionName}</h3>
            {items.map(({ key, value }) => (
                <ConfigItem
                    key={key}
                    configKey={key}
                    value={value}
                />
            ))}
        </div>
    );
};

export default ConfigSection;