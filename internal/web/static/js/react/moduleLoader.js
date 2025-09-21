// React ES6模块加载器 - 支持现代import/export语法
// 2025-09-20 21:15:29

console.log('🚀 初始化React ES6模块加载器...');

class ReactModuleLoader {
    constructor() {
        this.modules = new Map();
        this.pendingModules = new Map();
        this.baseUrl = '/static/js/react/';
        this.initialized = false;
    }

    // 初始化模块系统
    async initialize() {
        if (this.initialized) return;

        console.log('📦 [模块加载器] 初始化ES6模块系统...');

        // 确保React基础依赖就绪
        if (!window.React || !window.ReactDOM || !window.Babel) {
            throw new Error('React基础依赖未就绪');
        }

        this.initialized = true;
        console.log('✅ [模块加载器] ES6模块系统初始化完成');
    }

    // 动态import模拟器 - 支持JSX编译
    async importModule(modulePath) {
        try {
            console.log(`📥 [模块加载器] 加载模块: ${modulePath}`);

            // 检查缓存
            if (this.modules.has(modulePath)) {
                console.log(`🔄 [模块加载器] 使用缓存模块: ${modulePath}`);
                return this.modules.get(modulePath);
            }

            // 检查是否正在加载
            if (this.pendingModules.has(modulePath)) {
                console.log(`⏳ [模块加载器] 等待模块加载: ${modulePath}`);
                return await this.pendingModules.get(modulePath);
            }

            // 开始加载
            const loadPromise = this._loadModuleFile(modulePath);
            this.pendingModules.set(modulePath, loadPromise);

            const moduleExports = await loadPromise;

            // 缓存结果
            this.modules.set(modulePath, moduleExports);
            this.pendingModules.delete(modulePath);

            console.log(`✅ [模块加载器] 模块加载成功: ${modulePath}`);
            return moduleExports;

        } catch (error) {
            console.error(`❌ [模块加载器] 模块加载失败: ${modulePath}`, error);
            this.pendingModules.delete(modulePath);
            throw error;
        }
    }

    // 实际加载模块文件
    async _loadModuleFile(modulePath) {
        const fullUrl = this.baseUrl + modulePath;

        try {
            // 获取源码
            const response = await fetch(fullUrl);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            let sourceCode = await response.text();
            console.log(`📄 [模块加载器] 源码获取成功: ${modulePath} (${sourceCode.length} 字符)`);

            // 解析并预加载依赖（在Babel转换前进行，因为转换后import语句会消失）
            const dependencies = this._parseDependencies(sourceCode);
            const resolvedDependencies = dependencies.map(dep => this._resolveModulePath(dep, modulePath));
            await this._loadDependencies(resolvedDependencies);

            // 使用Babel进行JSX和ES6模块转换（统一处理所有.jsx文件）
            if (modulePath.endsWith('.jsx')) {
                sourceCode = this._transformWithBabel(sourceCode);
                console.log(`🔄 [模块加载器] Babel转换完成: ${modulePath}`);
            }

            // 创建模块执行环境
            const moduleExports = {};
            const moduleScope = {
                exports: moduleExports,
                module: { exports: moduleExports },
                React: window.React,
                ReactDOM: window.ReactDOM,
                console: console,
                window: window,
                document: document,
                fetch: window.fetch,
                setTimeout: window.setTimeout,
                setInterval: window.setInterval,
                clearTimeout: window.clearTimeout,
                clearInterval: window.clearInterval,
                // 实现 require 函数来支持 CommonJS
                require: (requirePath) => {
                    try {
                        console.log(`🔗 [require调用] 模块请求: ${requirePath} (来自 ${modulePath})`);

                        // 处理 React 相关导入
                        if (requirePath === 'react') {
                            return window.React;
                        }

                        // 解析相对路径
                        const resolvedPath = this._resolveModulePath(requirePath, modulePath);

                        // 从缓存中获取模块
                        const cachedModule = this.modules.get(resolvedPath);
                        if (cachedModule) {
                            console.log(`✅ [require调用] 缓存命中: ${resolvedPath}`);
                            return cachedModule;
                        }

                        // 如果模块还未加载，抛出错误（因为依赖应该已经预加载）
                        console.error(`❌ [require调用] 模块未找到: ${resolvedPath}`);
                        console.log(`📋 [require调用] 当前已加载模块:`, Array.from(this.modules.keys()));

                        // 返回空对象避免程序崩溃
                        return {};
                    } catch (error) {
                        console.error(`❌ [require调用] 模块加载错误: ${requirePath}`, error);
                        return {};
                    }
                }
            };

            // 执行模块代码
            const moduleFunction = new Function(
                ...Object.keys(moduleScope),
                sourceCode
            );

            moduleFunction(...Object.values(moduleScope));

            return moduleScope.module.exports;

        } catch (error) {
            console.error(`❌ [模块加载器] 编译执行失败: ${modulePath}`, error);
            throw error;
        }
    }

    // 解析依赖关系（支持多行import语句）
    _parseDependencies(code) {
        const dependencies = [];
        // 修复正则表达式，使用 [\s\S]*? 来匹配包括换行符在内的所有字符
        const importRegex = /import\s+([\s\S]*?)\s+from\s+['"`](.+?)['"`];?/g;
        let match;

        while ((match = importRegex.exec(code)) !== null) {
            const [, imports, modulePath] = match;
            if (!imports.includes('React') && !modulePath.startsWith('react')) {
                dependencies.push(modulePath);
            }
        }

        console.log(`🔗 [依赖解析] 发现 ${dependencies.length} 个依赖:`, dependencies);
        return dependencies;
    }

    // 解析相对路径
    _resolveModulePath(importPath, currentModulePath) {
        // 如果是相对路径，需要根据当前模块路径解析
        if (importPath.startsWith('./') || importPath.startsWith('../')) {
            const currentDir = currentModulePath.includes('/')
                ? currentModulePath.substring(0, currentModulePath.lastIndexOf('/'))
                : '';

            // 处理 ./
            if (importPath.startsWith('./')) {
                const resolvedPath = currentDir ? `${currentDir}/${importPath.slice(2)}` : importPath.slice(2);
                console.log(`🔄 [路径解析] ${importPath} (在 ${currentModulePath}) -> ${resolvedPath}`);
                return resolvedPath;
            }

            // 处理 ../
            if (importPath.startsWith('../')) {
                const pathParts = currentDir.split('/');
                const importParts = importPath.split('/');

                // 移除每个 ..
                for (const part of importParts) {
                    if (part === '..') {
                        pathParts.pop();
                    } else if (part !== '.') {
                        pathParts.push(part);
                    }
                }

                const resolvedPath = pathParts.join('/');
                console.log(`🔄 [路径解析] ${importPath} (在 ${currentModulePath}) -> ${resolvedPath}`);
                return resolvedPath;
            }
        }

        // 绝对路径直接返回
        return importPath;
    }

    // 预加载依赖
    async _loadDependencies(dependencies) {
        for (const dep of dependencies) {
            if (!this.modules.has(dep)) {
                console.log(`📦 [依赖加载] 预加载依赖: ${dep}`);
                await this.importModule(dep);
            }
        }
    }

    // JSX和ES6模块转换（使用Babel统一处理）
    _transformWithBabel(code) {
        try {
            const transformed = Babel.transform(code, {
                sourceType: 'module', // 告诉 Babel 输入是 ES Module
                presets: [
                    ['env', { modules: 'commonjs', targets: { esmodules: false } }],
                    'react'
                ]
            });
            console.log('✅ [Babel转换] JSX 和模块语法已完成转换');
            return transformed.code;
        } catch (error) {
            console.error('❌ [Babel转换] 转换失败:', error);
            throw error;
        }
    }

    // 获取模块状态
    getModuleStatus() {
        return {
            initialized: this.initialized,
            loadedModules: Array.from(this.modules.keys()),
            pendingModules: Array.from(this.pendingModules.keys()),
            totalLoaded: this.modules.size
        };
    }
}

// 创建全局模块加载器实例
window.ReactModuleLoader = new ReactModuleLoader();

// 提供便捷的import函数
window.importReactModule = async (modulePath) => {
    await window.ReactModuleLoader.initialize();
    return await window.ReactModuleLoader.importModule(modulePath);
};

console.log('✅ React ES6模块加载器已就绪');