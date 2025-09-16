// React ES6模块加载器 - 支持现代import/export语法
// 2025-09-15 15:58:44

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

            // 如果是JSX文件，进行Babel编译
            if (modulePath.endsWith('.jsx')) {
                sourceCode = this._transformJSX(sourceCode);
            }

            // 解析并预加载依赖
            const dependencies = this._parseDependencies(sourceCode);
            const resolvedDependencies = dependencies.map(dep => this._resolveModulePath(dep, modulePath));
            await this._loadDependencies(resolvedDependencies);

            // 转换import/export语法为CommonJS
            sourceCode = this._transformImportExport(sourceCode, resolvedDependencies, modulePath);

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
                clearInterval: window.clearInterval
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

    // 解析依赖关系
    _parseDependencies(code) {
        const dependencies = [];
        const importRegex = /import\s+(.*?)\s+from\s+['"`](.+?)['"`];?/g;
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

    // JSX转换
    _transformJSX(code) {
        try {
            const transformed = Babel.transform(code, {
                presets: ['react'],
                plugins: []
            });
            return transformed.code;
        } catch (error) {
            console.error('❌ [模块加载器] JSX编译失败:', error);
            throw error;
        }
    }

    // 完整的import/export转换
    _transformImportExport(code, resolvedDependencies = [], currentModulePath = '') {
        console.log('🔄 [模块转换] 开始转换import/export语法...');

        // 创建原路径到解析路径的映射
        const pathMap = new Map();
        const importRegex = /import\s+(.*?)\s+from\s+['"`](.+?)['"`];?/g;
        let match;
        let index = 0;

        // 先建立路径映射关系
        while ((match = importRegex.exec(code)) !== null) {
            const [, imports, originalPath] = match;
            if (!imports.includes('React') && !originalPath.startsWith('react')) {
                if (index < resolvedDependencies.length) {
                    pathMap.set(originalPath, resolvedDependencies[index]);
                    index++;
                }
            }
        }

        // 重置正则表达式
        importRegex.lastIndex = 0;

        // 1. 转换 import 语句
        code = code.replace(
            /import\s+(.*?)\s+from\s+['"`](.+?)['"`];?/g,
            (match, imports, modulePath) => {
                console.log(`📥 [模块转换] 转换import: ${imports} from ${modulePath}`);

                // 处理不同的import模式
                if (imports.includes('React') || modulePath === 'react') {
                    // 处理React相关导入
                    if (imports.startsWith('{') && imports.endsWith('}')) {
                        // import { useState, useEffect } from 'react'
                        const reactImports = imports.slice(1, -1).split(',').map(s => s.trim());
                        return reactImports.map(imp => `const ${imp} = React.${imp};`).join('\n');
                    } else if (imports === 'React') {
                        // import React from 'react' -> 已在全局可用
                        return `// React 已在模块环境中可用`;
                    } else if (imports.includes(',')) {
                        // import React, { useState, useEffect } from 'react'
                        const parts = imports.split(',').map(s => s.trim());
                        const defaultImport = parts[0];
                        const destructuredPart = parts.slice(1).join(',').trim();

                        let result = [];
                        if (defaultImport === 'React') {
                            result.push(`// React 已在模块环境中可用`);
                        }

                        // 处理解构部分
                        if (destructuredPart.startsWith('{') && destructuredPart.endsWith('}')) {
                            const reactImports = destructuredPart.slice(1, -1).split(',').map(s => s.trim());
                            result.push(...reactImports.map(imp => `const ${imp} = React.${imp};`));
                        }

                        return result.join('\n');
                    } else {
                        // 其他React相关导入
                        return `// React 相关导入已处理`;
                    }
                } else {
                    // 其他模块，从预加载的模块中获取
                    const resolvedPath = pathMap.get(modulePath) || modulePath;

                    if (imports.startsWith('{') && imports.endsWith('}')) {
                        // 解构导入 { Component1, Component2 }
                        const destructuredImports = imports.slice(1, -1).split(',').map(s => s.trim());
                        return `const { ${destructuredImports.join(', ')} } = (function() {
                            try {
                                const module = window.ReactModuleLoader.modules.get('${resolvedPath}');
                                return module.default || module;
                            } catch (e) {
                                console.error('模块获取失败:', '${resolvedPath}', e);
                                return {};
                            }
                        })();`;
                    } else {
                        // 默认导入 Component
                        const varName = imports.trim();
                        return `const ${varName} = (function() {
                            try {
                                const module = window.ReactModuleLoader.modules.get('${resolvedPath}');
                                return module.default || module;
                            } catch (e) {
                                console.error('模块获取失败:', '${resolvedPath}', e);
                                return null;
                            }
                        })();`;
                    }
                }
            }
        );

        // 2. 转换 export default
        code = code.replace(
            /export\s+default\s+(.+?)(?:;|$)/gm,
            'module.exports = $1;'
        );

        // 3. 转换 export { name }
        code = code.replace(
            /export\s*\{\s*([^}]+)\s*\}(?:;|$)/gm,
            (match, exports) => {
                const exportList = exports.split(',').map(e => e.trim());
                return exportList.map(exp => `module.exports.${exp} = ${exp};`).join('\n');
            }
        );

        // 4. 转换 export const/function/class
        code = code.replace(
            /export\s+(const|function|class)\s+(\w+)/g,
            (match, type, name) => {
                return `${type} ${name}; module.exports.${name} = ${name};`;
            }
        );

        console.log('✅ [模块转换] import/export转换完成');
        return code;
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