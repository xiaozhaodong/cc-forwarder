// Claude Request Forwarder - React组件注册系统
// 用于管理React组件的注册和访问，支持渐进式迁移

console.log('🚀 初始化React组件注册系统...');

// 全局React组件注册表
window.ReactComponents = {
    // 应急激活相关组件
    EmergencyDialog: null,
    ForceActivateButton: null,

    // 组管理相关组件
    GroupCard: null,
    GroupsList: null,

    // 图表相关组件
    RealtimeChart: null,

    // 通用组件
    LoadingSpinner: null,

    // React 18 根容器管理
    _roots: new Map(),

    // 内部状态管理
    _initialized: false,
    _loadingComponents: new Set(),
    _failedComponents: new Set(),

    // 注册组件
    register(name, component) {
        if (typeof component !== 'function') {
            console.error(`❌ [组件注册] 组件 ${name} 必须是一个函数`);
            this._failedComponents.add(name);
            return false;
        }

        this[name] = component;
        this._loadingComponents.delete(name);
        console.log(`✅ [组件注册] 成功注册组件: ${name}`);

        // 触发组件注册事件
        const event = new CustomEvent('reactComponentRegistered', {
            detail: { name, component }
        });
        document.dispatchEvent(event);

        return true;
    },

    // 检查组件是否可用
    isAvailable(name) {
        return this[name] && typeof this[name] === 'function';
    },

    // 获取组件
    get(name) {
        if (this.isAvailable(name)) {
            return this[name];
        }
        console.warn(`⚠️ [组件获取] 组件 ${name} 不可用，可能未注册或注册失败`);
        return null;
    },

    // React 18 兼容的渲染方法
    renderComponent(component, container) {
        if (!container) {
            console.error('❌ [React渲染] 容器不存在');
            return false;
        }

        try {
            // 检查是否支持React 18的createRoot
            if (window.ReactDOM.createRoot) {
                // React 18+ 新API
                let root = this._roots.get(container);
                if (!root) {
                    root = ReactDOM.createRoot(container);
                    this._roots.set(container, root);
                    console.log('✅ [React18] 创建新的Root容器');
                }
                root.render(component);
                console.log('✅ [React18] 使用createRoot渲染组件');
            } else {
                // React 17 兼容API
                ReactDOM.render(component, container);
                console.log('✅ [React17] 使用传统render渲染组件');
            }
            return true;
        } catch (error) {
            console.error('❌ [React渲染] 渲染失败:', error);
            return false;
        }
    },

    // 卸载组件
    unmountComponent(container) {
        if (!container) return false;

        try {
            const root = this._roots.get(container);
            if (root) {
                // React 18+ 卸载
                root.unmount();
                this._roots.delete(container);
                console.log('✅ [React18] 组件已卸载');
            } else if (window.ReactDOM.unmountComponentAtNode) {
                // React 17 兼容
                ReactDOM.unmountComponentAtNode(container);
                console.log('✅ [React17] 组件已卸载');
            } else {
                // 如果都没有，尝试清空容器
                container.innerHTML = '';
                console.log('✅ [备用] 容器已清空');
            }
            return true;
        } catch (error) {
            console.error('❌ [React卸载] 卸载失败:', error);
            // 出错时强制清空容器
            try {
                container.innerHTML = '';
                this._roots.delete(container);
                console.log('🔧 [强制清理] 容器已强制清空');
            } catch (cleanupError) {
                console.error('❌ [强制清理] 清理失败:', cleanupError);
            }
            return false;
        }
    },

    // 检查React是否就绪
    isReactReady() {
        return !window.reactLoadFailed &&
               window.React &&
               window.ReactDOM &&
               window.Babel;
    },

    // 获取系统状态
    getStatus() {
        const registeredComponents = Object.keys(this)
            .filter(key => !key.startsWith('_') && typeof this[key] === 'function')
            .filter(key => !['register', 'isAvailable', 'get', 'isReactReady', 'getStatus', 'renderComponent', 'unmountComponent'].includes(key));

        return {
            reactReady: this.isReactReady(),
            reactVersion: window.React?.version || 'unknown',
            hasCreateRoot: !!window.ReactDOM?.createRoot,
            initialized: this._initialized,
            registeredComponents,
            loadingComponents: Array.from(this._loadingComponents),
            failedComponents: Array.from(this._failedComponents),
            totalRegistered: registeredComponents.length,
            activeRoots: this._roots.size
        };
    }
};

// 初始化函数
function initializeReactSystem() {
    if (window.ReactComponents._initialized) {
        console.log('✅ [React系统] 已初始化，跳过重复初始化');
        return;
    }

    if (!window.ReactComponents.isReactReady()) {
        console.warn('⚠️ [React系统] React依赖未就绪，延迟初始化...');
        setTimeout(initializeReactSystem, 100);
        return;
    }

    window.ReactComponents._initialized = true;
    console.log('✅ [React系统] 初始化完成');
    console.log('📊 [React系统] 状态:', window.ReactComponents.getStatus());

    // 触发React系统就绪事件
    const event = new CustomEvent('reactSystemReady', {
        detail: window.ReactComponents.getStatus()
    });
    document.dispatchEvent(event);
}

// DOM加载完成后初始化
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeReactSystem);
} else {
    initializeReactSystem();
}

// 为开发调试提供全局访问
window.debugReactComponents = () => {
    console.log('🔍 [调试] React组件系统状态:', window.ReactComponents.getStatus());
    return window.ReactComponents.getStatus();
};

console.log('✅ React组件注册系统已加载');