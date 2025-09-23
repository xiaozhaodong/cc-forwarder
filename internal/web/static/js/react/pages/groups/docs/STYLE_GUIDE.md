# 组管理React组件样式使用文档

**项目**: Claude Request Forwarder
**版本**: v3.2.0+
**日期**: 2025年09月16日
**文档类型**: 样式使用指南和最佳实践

## 概述

本文档详细说明组管理React组件的样式系统，包括CSS类名规范、主题支持、响应式设计和最佳实践。所有样式都与传统JavaScript实现100%兼容，并在此基础上增加了现代化的增强效果。

## 目录

1. [核心CSS类名](#核心css类名)
2. [CSS变量系统](#css变量系统)
3. [组件样式映射](#组件样式映射)
4. [增强特性](#增强特性)
5. [主题支持](#主题支持)
6. [响应式设计](#响应式设计)
7. [最佳实践](#最佳实践)
8. [故障排除](#故障排除)

## 核心CSS类名

### 1. 组卡片相关类名

#### 基础容器
```css
.group-info-cards          /* 组卡片网格容器 */
.group-info-card           /* 单个组卡片基础样式 */
.group-card-header         /* 组卡片头部区域 */
```

#### 状态修饰类
```css
.group-info-card.active           /* 活跃组状态 */
.group-info-card.inactive         /* 非活跃组状态 */
.group-info-card.cooldown         /* 冷却中状态 */
.group-info-card.force-activated  /* 应急激活状态 [新增] */
```

#### 内容区域类名
```css
.group-name            /* 组名称 */
.group-status          /* 组状态指示器 */
.group-details         /* 组详细信息区域 */
.group-actions         /* 组操作按钮区域 */
```

### 2. 组状态指示器

#### 状态样式
```css
.group-status.active    /* 活跃状态徽章 */
.group-status.inactive  /* 非活跃状态徽章 */
.group-status.cooldown  /* 冷却状态徽章 */
```

### 3. 操作按钮类名

#### 基础按钮
```css
.group-btn                    /* 组操作按钮基础样式 */
.group-btn.btn-activate       /* 激活按钮 */
.group-btn.btn-pause          /* 暂停按钮 */
.group-btn.btn-resume         /* 恢复按钮 */
.group-btn.btn-force-activate /* 应急激活按钮 [新增] */
```

### 4. 确认对话框类名

#### 对话框结构
```css
.confirm-dialog-overlay       /* 对话框遮罩层 */
.confirm-dialog              /* 对话框容器 */
.confirm-dialog-header       /* 对话框头部 */
.confirm-dialog-icon         /* 对话框图标 */
.confirm-dialog-title        /* 对话框标题 */
.confirm-dialog-body         /* 对话框内容区域 */
.confirm-dialog-message      /* 主要消息文本 */
.confirm-dialog-details      /* 详细信息列表 */
.confirm-dialog-warning      /* 警告信息区域 */
.confirm-dialog-footer       /* 对话框底部按钮区域 */
```

#### 对话框按钮
```css
.confirm-dialog-btn                /* 对话框按钮基础样式 */
.confirm-dialog-btn-cancel         /* 取消按钮 */
.confirm-dialog-btn-confirm        /* 确认按钮 */
```

### 5. 其他功能类名

#### 冷却和状态信息
```css
.group-cooldown-info        /* 冷却时间信息 */
.group-force-activation-info /* 应急激活信息 [新增] */
```

#### 统计和警告
```css
.groups-container    /* 组统计容器 */
.groups-summary      /* 组统计概要 */
.alert-banner        /* 挂起请求警告横幅 */
```

## CSS变量系统

### 1. 基础色彩变量

#### 主色调
```css
--primary-color: #2563eb     /* 主题蓝色 */
--secondary-color: #64748b   /* 次要灰色 */
--success-color: #10b981     /* 成功绿色 */
--error-color: #ef4444       /* 错误红色 */
--warning-color: #f59e0b     /* 警告黄色 */
```

#### 背景和文本
```css
--bg-color: #f8fafc          /* 页面背景色 */
--card-bg: #ffffff           /* 卡片背景色 */
--border-color: #e2e8f0      /* 边框颜色 */
--text-color: #1e293b        /* 主文本颜色 */
--text-muted: #64748b        /* 次要文本颜色 */
```

### 2. 增强效果变量 [新增]

#### 阴影效果
```css
--shadow-light: rgba(0, 0, 0, 0.1)    /* 轻微阴影 */
--shadow-medium: rgba(0, 0, 0, 0.15)  /* 中等阴影 */
--shadow-heavy: rgba(0, 0, 0, 0.2)    /* 重阴影 */
```

#### 交互效果
```css
--overlay-bg: rgba(0, 0, 0, 0.5)      /* 遮罩背景 */
--glass-bg: rgba(255, 255, 255, 0.9)  /* 毛玻璃效果 */
--hover-bg: #f1f5f9                   /* 悬停背景 */
```

## 组件样式映射

### 1. GroupCard 组件

#### HTML结构
```jsx
<div className={`group-info-card ${statusClass}`}>
  <div className="group-card-header">
    <h3 className="group-name">{name}</h3>
    <GroupStatus />
  </div>
  <GroupDetails />
  <GroupActions />
  {/* 条件渲染的状态信息 */}
</div>
```

#### 状态类映射
```javascript
const getGroupStatusClass = (group) => {
  if (group.in_cooldown) return 'cooldown';
  if (group.is_force_activated) return 'force-activated';  // 新增
  if (group.is_active) return 'active';
  return 'inactive';
};
```

### 2. GroupStatus 组件

#### 状态徽章结构
```jsx
<span className={`group-status ${statusClass}`}>
  {statusText}
</span>
```

### 3. GroupActions 组件

#### 按钮布局
```jsx
<div className="group-actions">
  <button className="group-btn btn-activate">激活</button>
  <button className="group-btn btn-pause">暂停</button>
  <button className="group-btn btn-resume">恢复</button>
  <button className="group-btn btn-force-activate">应急激活</button>
</div>
```

### 4. ConfirmDialog 组件

#### 完整对话框结构
```jsx
<div className="confirm-dialog-overlay show">
  <div className="confirm-dialog">
    <div className="confirm-dialog-header">
      <div className="confirm-dialog-icon">⚠️</div>
      <h3 className="confirm-dialog-title">确认操作</h3>
    </div>
    <div className="confirm-dialog-body">
      <div className="confirm-dialog-message">消息内容</div>
      <div className="confirm-dialog-details">详细信息</div>
      <div className="confirm-dialog-warning">警告信息</div>
    </div>
    <div className="confirm-dialog-footer">
      <button className="confirm-dialog-btn confirm-dialog-btn-cancel">取消</button>
      <button className="confirm-dialog-btn confirm-dialog-btn-confirm">确认</button>
    </div>
  </div>
</div>
```

## 增强特性

### 1. 视觉增强效果

#### 组卡片顶部渐变线条 [新增]
```css
.group-info-card::before {
  /* 顶部4px高度的渐变线条，根据组状态显示不同颜色 */
}
```

#### 应急激活脉冲动画 [新增]
```css
@keyframes emergencyPulse {
  /* 应急激活组的呼吸灯效果 */
}
```

### 2. 交互增强效果

#### 按钮波纹点击效果 [新增]
```css
.group-btn::after {
  /* Material Design风格的点击波纹效果 */
}
```

#### 应急按钮脉冲提醒 [新增]
```css
@keyframes emergencyButtonPulse {
  /* 应急激活按钮的脉冲动画提醒 */
}
```

### 3. 高级视觉效果

#### 渐变背景
- 所有按钮使用 `linear-gradient` 渐变背景
- 应急激活组使用特殊的渐变背景效果

#### 动态阴影
- 悬停时阴影加深和位移效果
- 不同状态的组使用对应颜色的阴影

## 主题支持

### 1. 自动暗色主题
```css
@media (prefers-color-scheme: dark) {
  /* 根据系统设置自动切换暗色主题 */
}
```

### 2. 手动主题切换
```css
.dark-theme {
  /* 通过添加此类名手动启用暗色主题 */
}
```

### 3. 主题切换示例
```javascript
// 切换到暗色主题
document.body.classList.add('dark-theme');

// 切换到亮色主题
document.body.classList.remove('dark-theme');
```

## 响应式设计

### 1. 断点设计

#### 桌面端 (>1200px)
```css
.group-info-cards {
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}
```

#### 平板端 (768px - 1200px)
```css
@media (max-width: 1200px) {
  .group-info-cards {
    gap: 15px;
  }
}
```

#### 移动端 (<768px)
```css
@media (max-width: 768px) {
  .group-info-cards {
    grid-template-columns: 1fr;
  }
  .group-actions {
    flex-direction: column;
  }
}
```

### 2. 组件响应式行为

#### 组卡片布局
- **桌面**: 多列网格布局，卡片宽度自适应
- **平板**: 减小间距，保持多列布局
- **移动**: 单列堆叠，按钮垂直排列

#### 确认对话框
- **桌面**: 固定宽度居中显示
- **移动**: 全宽度显示，适配小屏幕

## 最佳实践

### 1. 类名使用规范

#### ✅ 正确用法
```jsx
// 使用完整的状态类名
<div className={`group-info-card ${getGroupStatusClass(group)}`}>

// 保持原有的CSS类名结构
<button className="group-btn btn-activate">
```

#### ❌ 错误用法
```jsx
// 不要创建新的类名
<div className="custom-group-card">

// 不要省略基础类名
<button className="btn-activate">
```

### 2. 状态管理最佳实践

#### 组状态映射
```javascript
// 确保状态映射的完整性和准确性
const statusMapping = {
  active: '活跃',
  inactive: '待激活',
  cooldown: '冷却中',
  'force-activated': '应急状态'  // 新增状态
};
```

### 3. 动画性能优化

#### 使用 transform 而非位置属性
```css
/* ✅ 性能良好 */
.group-info-card:hover {
  transform: translateY(-2px);
}

/* ❌ 性能较差 */
.group-info-card:hover {
  top: -2px;
}
```

### 4. CSS变量使用

#### 颜色引用
```css
/* ✅ 使用CSS变量，支持主题切换 */
background: var(--success-color);

/* ❌ 硬编码颜色值 */
background: #10b981;
```

## 故障排除

### 1. 常见问题

#### 问题：组卡片状态样式不生效
**解决方案**：
1. 检查状态类名是否正确应用
2. 确认 `getGroupStatusClass` 函数返回值
3. 验证CSS文件是否正确加载

#### 问题：确认对话框样式异常
**解决方案**：
1. 检查 `confirm-dialog-overlay.show` 类名
2. 确认对话框HTML结构完整
3. 验证z-index层级设置

#### 问题：暗色主题不生效
**解决方案**：
1. 检查浏览器是否支持 `prefers-color-scheme`
2. 手动添加 `.dark-theme` 类测试
3. 确认CSS变量定义顺序

### 2. 调试技巧

#### 使用浏览器开发工具
```javascript
// 检查CSS变量值
getComputedStyle(document.documentElement)
  .getPropertyValue('--primary-color');

// 动态切换主题测试
document.body.classList.toggle('dark-theme');
```

#### 样式覆盖检查
```css
/* 临时添加重要性检查样式冲突 */
.group-info-card {
  border: 2px solid red !important;
}
```

### 3. 性能监控

#### 动画性能检查
```javascript
// 监控重排和重绘
performance.mark('animation-start');
// 触发动画
performance.mark('animation-end');
performance.measure('animation-duration', 'animation-start', 'animation-end');
```

## 版本更新记录

### v3.2.0+ (2025-09-16)
- ✅ 新增 `.force-activated` 状态类和样式
- ✅ 新增应急激活按钮 `.btn-force-activate`
- ✅ 新增脉冲动画效果
- ✅ 新增按钮波纹点击效果
- ✅ 新增CSS变量系统扩展
- ✅ 新增暗色主题支持
- ✅ 增强确认对话框样式兼容性

### 兼容性说明
- ✅ 100%向下兼容传统JavaScript实现
- ✅ 支持现代浏览器的新特性
- ✅ 渐进式增强，旧浏览器降级使用基础样式

## 总结

本样式系统为组管理React组件提供了：

1. **完全兼容性** - 与传统实现100%样式一致
2. **现代化增强** - 添加了渐变、动画、波纹等现代效果
3. **主题支持** - 自动和手动暗色主题切换
4. **响应式设计** - 适配各种屏幕尺寸
5. **性能优化** - 使用CSS3硬件加速动画
6. **可维护性** - 基于CSS变量的主题系统

通过遵循本文档的指导原则，可以确保组管理React组件的样式表现稳定、一致且具有良好的用户体验。