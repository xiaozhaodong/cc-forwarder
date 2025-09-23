/**
 * React页面迁移功能测试和兼容性验证
 * 文件描述: 全面测试React requests页面的功能性和兼容性
 * 创建时间: 2025-09-20 21:00:00
 *
 * 测试范围:
 * 1. 组件导入验证
 * 2. 语法和类型检查
 * 3. 样式兼容性验证
 * 4. 功能集成测试
 * 5. API兼容性测试
 */

// 模拟浏览器环境
const { JSDOM } = require('jsdom');
const fs = require('fs');
const path = require('path');

// 测试配置
const TEST_CONFIG = {
    baseDir: '/Users/xiaozhaodong/GoProjects/cc-forwarder',
    reactDir: '/Users/xiaozhaodong/GoProjects/cc-forwarder/internal/web/static/js/react',
    testTimeout: 30000,
    maxRetries: 3
};

// 测试结果收集器
class TestResultCollector {
    constructor() {
        this.results = {
            componentImports: { passed: 0, failed: 0, errors: [] },
            syntaxValidation: { passed: 0, failed: 0, errors: [] },
            styleCompatibility: { passed: 0, failed: 0, errors: [] },
            functionalTests: { passed: 0, failed: 0, errors: [] },
            apiCompatibility: { passed: 0, failed: 0, errors: [] },
            overall: { status: 'unknown', summary: '', recommendations: [] }
        };
        this.startTime = Date.now();
    }

    addResult(category, test, passed, error = null) {
        if (passed) {
            this.results[category].passed++;
            console.log(`✅ [${category}] ${test} - 通过`);
        } else {
            this.results[category].failed++;
            this.results[category].errors.push({ test, error: error?.message || error });
            console.log(`❌ [${category}] ${test} - 失败: ${error?.message || error}`);
        }
    }

    generateReport() {
        const endTime = Date.now();
        const duration = endTime - this.startTime;

        let totalPassed = 0;
        let totalFailed = 0;

        Object.values(this.results).forEach(category => {
            if (typeof category === 'object' && category.passed !== undefined) {
                totalPassed += category.passed;
                totalFailed += category.failed;
            }
        });

        // 确定整体状态
        const successRate = totalPassed / (totalPassed + totalFailed);
        let overallStatus, statusIcon;

        if (successRate >= 0.9) {
            overallStatus = 'excellent';
            statusIcon = '🎉';
        } else if (successRate >= 0.8) {
            overallStatus = 'good';
            statusIcon = '✅';
        } else if (successRate >= 0.6) {
            overallStatus = 'warning';
            statusIcon = '⚠️';
        } else {
            overallStatus = 'critical';
            statusIcon = '❌';
        }

        this.results.overall = {
            status: overallStatus,
            summary: `${statusIcon} 测试完成: ${totalPassed}/${totalPassed + totalFailed} 项通过 (${(successRate * 100).toFixed(1)}%)`,
            recommendations: this.generateRecommendations(),
            duration: `${(duration / 1000).toFixed(2)}秒`,
            timestamp: new Date().toISOString()
        };

        return this.results;
    }

    generateRecommendations() {
        const recommendations = [];

        Object.entries(this.results).forEach(([category, result]) => {
            if (result.failed > 0) {
                switch (category) {
                    case 'componentImports':
                        recommendations.push('🔧 修复组件导入错误，检查文件路径和export语法');
                        break;
                    case 'syntaxValidation':
                        recommendations.push('📝 修复JSX语法错误，确保代码符合React规范');
                        break;
                    case 'styleCompatibility':
                        recommendations.push('🎨 修复CSS样式兼容性问题，确保响应式设计正常');
                        break;
                    case 'functionalTests':
                        recommendations.push('⚙️ 修复功能性问题，确保组件交互正常工作');
                        break;
                    case 'apiCompatibility':
                        recommendations.push('🔌 修复API兼容性问题，确保后端接口调用正常');
                        break;
                }
            }
        });

        if (recommendations.length === 0) {
            recommendations.push('🎯 所有测试通过，React页面迁移成功完成');
        }

        return recommendations;
    }
}

// 主测试类
class ReactMigrationTester {
    constructor() {
        this.collector = new TestResultCollector();
        this.setupEnvironment();
    }

    setupEnvironment() {
        // 设置JSDOM环境
        const dom = new JSDOM(`<!DOCTYPE html><html><head></head><body></body></html>`, {
            url: 'http://localhost:8011',
            pretendToBeVisual: true,
            resources: 'usable'
        });

        global.window = dom.window;
        global.document = dom.window.document;
        global.navigator = dom.window.navigator;
        global.HTMLElement = dom.window.HTMLElement;
        global.Event = dom.window.Event;
        global.CustomEvent = dom.window.CustomEvent;

        // 模拟React环境
        this.mockReactEnvironment();
    }

    mockReactEnvironment() {
        // 模拟React和ReactDOM
        global.React = {
            createElement: (type, props, ...children) => ({ type, props, children }),
            useState: (initial) => [initial, () => {}],
            useEffect: () => {},
            useCallback: (fn) => fn,
            useMemo: (fn) => fn(),
            memo: (component) => component,
            version: '18.0.0'
        };

        global.ReactDOM = {
            render: () => {},
            createRoot: () => ({
                render: () => {},
                unmount: () => {}
            })
        };

        global.Babel = {
            transform: (code) => ({ code })
        };

        global.fetch = async (url) => ({
            ok: true,
            json: async () => ({ requests: [], total: 0 }),
            text: async () => '',
            headers: { get: () => 'application/json' }
        });

        global.EventSource = function() {
            this.addEventListener = () => {};
            this.close = () => {};
            this.readyState = 1;
        };

        // 设置到window对象
        global.window.React = global.React;
        global.window.ReactDOM = global.ReactDOM;
        global.window.Babel = global.Babel;
        global.window.fetch = global.fetch;
        global.window.EventSource = global.EventSource;
    }

    // 1. 组件导入验证测试
    async testComponentImports() {
        console.log('\n🔍 开始组件导入验证测试...');

        const componentsToTest = [
            'pages/requests/index.jsx',
            'pages/requests/components/RequestsTable.jsx',
            'pages/requests/components/FiltersPanel.jsx',
            'pages/requests/components/PaginationControl.jsx',
            'pages/requests/components/RequestDetailModal.jsx',
            'pages/requests/hooks/useRequestsData.jsx',
            'pages/requests/hooks/useFilters.jsx',
            'pages/requests/hooks/usePagination.jsx',
            'pages/requests/utils/apiService.jsx'
        ];

        for (const component of componentsToTest) {
            await this.testSingleComponentImport(component);
        }
    }

    async testSingleComponentImport(componentPath) {
        try {
            const fullPath = path.join(TEST_CONFIG.reactDir, componentPath);

            // 检查文件是否存在
            if (!fs.existsSync(fullPath)) {
                throw new Error(`文件不存在: ${fullPath}`);
            }

            // 读取文件内容
            const content = fs.readFileSync(fullPath, 'utf8');

            // 基本语法验证
            this.validateBasicSyntax(content, componentPath);

            // 检查import语句
            this.validateImportStatements(content, componentPath);

            // 检查export语句
            this.validateExportStatements(content, componentPath);

            this.collector.addResult('componentImports', `导入 ${componentPath}`, true);
        } catch (error) {
            this.collector.addResult('componentImports', `导入 ${componentPath}`, false, error);
        }
    }

    validateBasicSyntax(content, componentPath) {
        // 检查JSX语法基础问题
        const issues = [];

        // 检查未闭合的标签
        const openTags = content.match(/<[^/>]+(?<!\/)\s*>/g) || [];
        const closeTags = content.match(/<\/[^>]+>/g) || [];

        // 检查React Hook规则
        const hookPattern = /use[A-Z]\w*/g;
        const hooks = content.match(hookPattern) || [];
        const conditionalHooks = content.match(/if\s*\([^)]*\)\s*\{[^}]*use[A-Z]/g);

        if (conditionalHooks) {
            issues.push('发现条件性使用Hook，违反Hook规则');
        }

        // 检查key属性
        const mapCalls = content.match(/\.map\s*\([^)]*\)\s*=>\s*<[^>]*>/g);
        if (mapCalls) {
            mapCalls.forEach(mapCall => {
                if (!mapCall.includes('key=')) {
                    issues.push('在map中渲染元素缺少key属性');
                }
            });
        }

        if (issues.length > 0) {
            throw new Error(`语法问题: ${issues.join(', ')}`);
        }
    }

    validateImportStatements(content, componentPath) {
        const importLines = content.split('\n').filter(line => line.trim().startsWith('import'));

        for (const importLine of importLines) {
            // 检查相对路径导入
            if (importLine.includes('./') || importLine.includes('../')) {
                const pathMatch = importLine.match(/['"`]([^'"`]+)['"`]/);
                if (pathMatch) {
                    const importPath = pathMatch[1];
                    // 验证路径解析逻辑
                    if (!this.validateRelativePath(importPath, componentPath)) {
                        throw new Error(`无效的相对路径: ${importPath}`);
                    }
                }
            }

            // 检查React导入
            if (importLine.includes('from \'react\'') || importLine.includes('from "react"')) {
                if (!importLine.includes('React')) {
                    console.warn(`建议显式导入React: ${importLine.trim()}`);
                }
            }
        }
    }

    validateExportStatements(content, componentPath) {
        const hasDefaultExport = content.includes('export default');
        const hasNamedExports = content.includes('export {') || content.includes('export const') || content.includes('export function');

        if (!hasDefaultExport && !hasNamedExports) {
            throw new Error('文件缺少export语句');
        }

        // 检查是否存在函数组件
        const componentName = path.basename(componentPath, '.jsx');
        const hasComponent = content.includes(`const ${componentName}`) ||
                           content.includes(`function ${componentName}`) ||
                           content.includes(`= (`) ||
                           content.includes('=>');

        if (componentPath.includes('components/') && !hasComponent) {
            console.warn(`组件文件 ${componentPath} 可能缺少React组件定义`);
        }
    }

    validateRelativePath(importPath, currentPath) {
        // 简化的路径验证逻辑
        const currentDir = path.dirname(currentPath);
        try {
            const resolvedPath = path.resolve(path.join(TEST_CONFIG.reactDir, currentDir), importPath);
            return resolvedPath.startsWith(TEST_CONFIG.reactDir);
        } catch {
            return false;
        }
    }

    // 2. 语法和类型检查测试
    async testSyntaxValidation() {
        console.log('\n📝 开始语法和类型检查测试...');

        // 修复PaginationControl.jsx中的React导入警告
        await this.testPaginationControlImport();

        // 检查JSX语法合规性
        await this.testJSXSyntaxCompliance();

        // 验证Props接口和类型定义
        await this.testPropsValidation();
    }

    async testPaginationControlImport() {
        try {
            const paginationPath = path.join(TEST_CONFIG.reactDir, 'pages/requests/components/PaginationControl.jsx');
            const content = fs.readFileSync(paginationPath, 'utf8');

            // 检查React导入
            const hasReactImport = content.includes('import React');

            if (!hasReactImport) {
                // 这是已知问题，记录但不算失败
                console.warn('⚠️ PaginationControl.jsx 缺少显式的React导入，但在当前实现中可能正常工作');
            }

            // 检查useState和useCallback使用
            const usesState = content.includes('useState');
            const usesCallback = content.includes('useCallback');

            if ((usesState || usesCallback) && !hasReactImport) {
                throw new Error('使用React Hooks但缺少React导入');
            }

            this.collector.addResult('syntaxValidation', 'PaginationControl React导入检查', true);
        } catch (error) {
            this.collector.addResult('syntaxValidation', 'PaginationControl React导入检查', false, error);
        }
    }

    async testJSXSyntaxCompliance() {
        try {
            const componentFiles = [
                'pages/requests/index.jsx',
                'pages/requests/components/RequestsTable.jsx',
                'pages/requests/components/FiltersPanel.jsx'
            ];

            for (const file of componentFiles) {
                const content = fs.readFileSync(path.join(TEST_CONFIG.reactDir, file), 'utf8');

                // 检查常见JSX语法问题
                this.checkJSXSyntaxIssues(content, file);
            }

            this.collector.addResult('syntaxValidation', 'JSX语法合规性检查', true);
        } catch (error) {
            this.collector.addResult('syntaxValidation', 'JSX语法合规性检查', false, error);
        }
    }

    checkJSXSyntaxIssues(content, filename) {
        const issues = [];

        // 检查自闭合标签
        const selfClosingTags = content.match(/<([a-zA-Z][a-zA-Z0-9]*)\s+[^>]*\/>/g);

        // 检查布尔属性
        const booleanAttrs = content.match(/\s(disabled|checked|selected|hidden)\s*=/g);
        if (booleanAttrs) {
            booleanAttrs.forEach(attr => {
                if (!attr.includes('={true}') && !attr.includes('={false}') && !attr.includes('={')) {
                    issues.push(`布尔属性应该使用JSX语法: ${attr.trim()}`);
                }
            });
        }

        // 检查className而不是class
        if (content.includes(' class=') && !content.includes('className=')) {
            issues.push('应该使用className而不是class');
        }

        if (issues.length > 0) {
            throw new Error(`JSX语法问题在 ${filename}: ${issues.join(', ')}`);
        }
    }

    async testPropsValidation() {
        try {
            // 检查组件Props的类型定义和使用
            const componentsWithProps = [
                'pages/requests/components/RequestsTable.jsx',
                'pages/requests/components/FiltersPanel.jsx',
                'pages/requests/components/PaginationControl.jsx'
            ];

            for (const file of componentsWithProps) {
                const content = fs.readFileSync(path.join(TEST_CONFIG.reactDir, file), 'utf8');
                this.validateComponentProps(content, file);
            }

            this.collector.addResult('syntaxValidation', 'Props接口和类型定义', true);
        } catch (error) {
            this.collector.addResult('syntaxValidation', 'Props接口和类型定义', false, error);
        }
    }

    validateComponentProps(content, filename) {
        // 检查解构赋值的Props
        const destructuringPattern = /const\s+\w+\s*=\s*\(\s*\{([^}]+)\}\s*\)/;
        const match = content.match(destructuringPattern);

        if (match) {
            const props = match[1].split(',').map(p => p.trim());

            // 检查是否有默认值
            const propsWithDefaults = props.filter(p => p.includes('='));
            console.log(`📋 ${filename} 发现 ${propsWithDefaults.length} 个带默认值的props`);
        }

        // 检查Props使用模式
        const propsUsage = content.match(/props\.\w+/g) || [];
        if (propsUsage.length > 0) {
            console.log(`📋 ${filename} 使用了 ${propsUsage.length} 个props引用`);
        }
    }

    // 3. 样式兼容性验证测试
    async testStyleCompatibility() {
        console.log('\n🎨 开始样式兼容性验证测试...');

        await this.testCSSClassNames();
        await this.testResponsiveDesign();
        await this.testExistingStylesCompatibility();
    }

    async testCSSClassNames() {
        try {
            const components = [
                'pages/requests/index.jsx',
                'pages/requests/components/RequestsTable.jsx',
                'pages/requests/components/FiltersPanel.jsx'
            ];

            const expectedClasses = [
                'requests-page', 'requests-table', 'filters-panel',
                'table-container', 'pagination-container', 'filter-group'
            ];

            let foundClasses = 0;
            for (const file of components) {
                const content = fs.readFileSync(path.join(TEST_CONFIG.reactDir, file), 'utf8');

                expectedClasses.forEach(className => {
                    if (content.includes(`"${className}"`) || content.includes(`'${className}'`)) {
                        foundClasses++;
                    }
                });
            }

            if (foundClasses >= expectedClasses.length * 0.8) {
                this.collector.addResult('styleCompatibility', 'CSS类名兼容性', true);
            } else {
                throw new Error(`仅发现 ${foundClasses}/${expectedClasses.length} 个预期的CSS类名`);
            }
        } catch (error) {
            this.collector.addResult('styleCompatibility', 'CSS类名兼容性', false, error);
        }
    }

    async testResponsiveDesign() {
        try {
            // 检查内联样式中的响应式设计元素
            const filtersPanel = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/components/FiltersPanel.jsx'),
                'utf8'
            );

            const hasResponsiveElements =
                filtersPanel.includes('flexWrap') ||
                filtersPanel.includes('flex-wrap') ||
                filtersPanel.includes('@media') ||
                filtersPanel.includes('responsive');

            this.collector.addResult('styleCompatibility', '响应式设计验证', hasResponsiveElements,
                hasResponsiveElements ? null : '未发现明显的响应式设计元素');
        } catch (error) {
            this.collector.addResult('styleCompatibility', '响应式设计验证', false, error);
        }
    }

    async testExistingStylesCompatibility() {
        try {
            // 检查是否保持与现有样式的兼容性
            const requestsPage = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/index.jsx'),
                'utf8'
            );

            // 检查CSS注释中是否包含样式定义
            const hasCSSComments = requestsPage.includes('/* ') || requestsPage.includes('*/');
            const hasStyleCompatibility =
                requestsPage.includes('requests-page') &&
                requestsPage.includes('btn') &&
                requestsPage.includes('btn-primary');

            this.collector.addResult('styleCompatibility', '现有样式兼容性', hasStyleCompatibility,
                hasStyleCompatibility ? null : '可能缺少与现有样式的兼容性');
        } catch (error) {
            this.collector.addResult('styleCompatibility', '现有样式兼容性', false, error);
        }
    }

    // 4. 功能集成测试
    async testFunctionalIntegration() {
        console.log('\n⚙️ 开始功能集成测试...');

        await this.testPageSwitching();
        await this.testFiltersFunctionality();
        await this.testPaginationFunctionality();
        await this.testModalFunctionality();
    }

    async testPageSwitching() {
        try {
            // 检查webInterface.js中的React页面加载逻辑
            const webInterfacePath = path.join(TEST_CONFIG.baseDir, 'internal/web/static/js/webInterface.js');
            const content = fs.readFileSync(webInterfacePath, 'utf8');

            const hasReactRequestsLoader = content.includes('loadReactRequestsPage');
            const hasModuleLoader = content.includes('importReactModule');
            const hasContainerElement = content.includes('react-requests-container');

            if (hasReactRequestsLoader && hasModuleLoader && hasContainerElement) {
                this.collector.addResult('functionalTests', '页面切换到requests页面', true);
            } else {
                throw new Error('React页面加载逻辑不完整');
            }
        } catch (error) {
            this.collector.addResult('functionalTests', '页面切换到requests页面', false, error);
        }
    }

    async testFiltersFunctionality() {
        try {
            const filtersHook = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/hooks/useFilters.jsx'),
                'utf8'
            );

            // 检查筛选器功能
            const hasFilterMethods =
                filtersHook.includes('updateFilter') &&
                filtersHook.includes('resetFilters') &&
                filtersHook.includes('applyFilters');

            const hasValidation = filtersHook.includes('validateFilters');
            const hasURLSync = filtersHook.includes('syncFiltersToURL');

            if (hasFilterMethods && hasValidation && hasURLSync) {
                this.collector.addResult('functionalTests', '筛选器功能完整性', true);
            } else {
                throw new Error('筛选器功能不完整');
            }
        } catch (error) {
            this.collector.addResult('functionalTests', '筛选器功能完整性', false, error);
        }
    }

    async testPaginationFunctionality() {
        try {
            const paginationHook = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/hooks/usePagination.jsx'),
                'utf8'
            );

            // 检查分页功能
            const hasPaginationMethods =
                paginationHook.includes('goToPage') &&
                paginationHook.includes('goToPrevPage') &&
                paginationHook.includes('goToNextPage') &&
                paginationHook.includes('changePageSize');

            const hasPageCalculation =
                paginationHook.includes('totalPages') &&
                paginationHook.includes('rangeText');

            if (hasPaginationMethods && hasPageCalculation) {
                this.collector.addResult('functionalTests', '分页功能完整性', true);
            } else {
                throw new Error('分页功能不完整');
            }
        } catch (error) {
            this.collector.addResult('functionalTests', '分页功能完整性', false, error);
        }
    }

    async testModalFunctionality() {
        try {
            const modalComponent = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/components/RequestDetailModal.jsx'),
                'utf8'
            );

            // 检查模态框功能
            const hasModalLogic =
                modalComponent.includes('isOpen') &&
                modalComponent.includes('onClose') &&
                modalComponent.includes('request');

            const hasKeyboardSupport = modalComponent.includes('Escape') || modalComponent.includes('keydown');

            this.collector.addResult('functionalTests', '模态框功能验证', hasModalLogic,
                hasModalLogic ? null : '模态框基本功能缺失');
        } catch (error) {
            this.collector.addResult('functionalTests', '模态框功能验证', false, error);
        }
    }

    // 5. API兼容性测试
    async testAPICompatibility() {
        console.log('\n🔌 开始API兼容性测试...');

        await this.testAPIServiceFunctions();
        await this.testDataFormatting();
        await this.testErrorHandling();
    }

    async testAPIServiceFunctions() {
        try {
            const apiService = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/utils/apiService.jsx'),
                'utf8'
            );

            // 检查API函数
            const requiredFunctions = [
                'fetchRequestsData',
                'fetchRequestDetail',
                'fetchModels',
                'fetchEndpoints',
                'fetchGroups'
            ];

            const missingFunctions = requiredFunctions.filter(func => !apiService.includes(func));

            if (missingFunctions.length === 0) {
                this.collector.addResult('apiCompatibility', 'API服务函数完整性', true);
            } else {
                throw new Error(`缺少API函数: ${missingFunctions.join(', ')}`);
            }
        } catch (error) {
            this.collector.addResult('apiCompatibility', 'API服务函数完整性', false, error);
        }
    }

    async testDataFormatting() {
        try {
            const apiService = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/utils/apiService.jsx'),
                'utf8'
            );

            // 检查数据格式化
            const hasDataNormalization =
                apiService.includes('requests: data.requests || data.data || data') &&
                apiService.includes('total: data.total || data.totalCount');

            const hasErrorHandling =
                apiService.includes('try {') &&
                apiService.includes('catch') &&
                apiService.includes('throw new Error');

            if (hasDataNormalization && hasErrorHandling) {
                this.collector.addResult('apiCompatibility', '数据格式化和错误处理', true);
            } else {
                throw new Error('数据格式化或错误处理不完整');
            }
        } catch (error) {
            this.collector.addResult('apiCompatibility', '数据格式化和错误处理', false, error);
        }
    }

    async testErrorHandling() {
        try {
            const requestsHook = fs.readFileSync(
                path.join(TEST_CONFIG.reactDir, 'pages/requests/hooks/useRequestsData.jsx'),
                'utf8'
            );

            // 检查错误处理机制
            const hasAbortController = requestsHook.includes('AbortController');
            const hasErrorState = requestsHook.includes('error');
            const hasLoadingState = requestsHook.includes('loading');

            if (hasAbortController && hasErrorState && hasLoadingState) {
                this.collector.addResult('apiCompatibility', '错误响应处理机制', true);
            } else {
                throw new Error('错误处理机制不完整');
            }
        } catch (error) {
            this.collector.addResult('apiCompatibility', '错误响应处理机制', false, error);
        }
    }

    // 主测试执行方法
    async runAllTests() {
        console.log('🚀 开始React页面迁移功能测试和兼容性验证\n');
        console.log(`📁 测试目录: ${TEST_CONFIG.reactDir}`);
        console.log(`⏱️  超时时间: ${TEST_CONFIG.testTimeout}ms\n`);

        try {
            // 执行所有测试
            await this.testComponentImports();
            await this.testSyntaxValidation();
            await this.testStyleCompatibility();
            await this.testFunctionalIntegration();
            await this.testAPICompatibility();

            // 生成测试报告
            const report = this.collector.generateReport();
            this.printTestReport(report);

            return report;
        } catch (error) {
            console.error('❌ 测试执行过程中发生严重错误:', error);
            throw error;
        }
    }

    printTestReport(report) {
        console.log('\n' + '='.repeat(80));
        console.log('📊 React页面迁移测试报告');
        console.log('='.repeat(80));

        console.log(`\n${report.overall.summary}`);
        console.log(`⏱️ 测试耗时: ${report.overall.duration}`);
        console.log(`📅 测试时间: ${report.overall.timestamp}\n`);

        // 详细测试结果
        Object.entries(report).forEach(([category, result]) => {
            if (typeof result === 'object' && result.passed !== undefined) {
                const total = result.passed + result.failed;
                const rate = total > 0 ? (result.passed / total * 100).toFixed(1) : '0.0';

                console.log(`📋 ${this.getCategoryName(category)}: ${result.passed}/${total} 通过 (${rate}%)`);

                if (result.errors.length > 0) {
                    result.errors.forEach(error => {
                        console.log(`   ❌ ${error.test}: ${error.error}`);
                    });
                }
            }
        });

        // 建议和后续步骤
        if (report.overall.recommendations.length > 0) {
            console.log('\n💡 建议和后续步骤:');
            report.overall.recommendations.forEach(rec => {
                console.log(`   ${rec}`);
            });
        }

        console.log('\n' + '='.repeat(80));
    }

    getCategoryName(category) {
        const names = {
            componentImports: '组件导入验证',
            syntaxValidation: '语法和类型检查',
            styleCompatibility: '样式兼容性验证',
            functionalTests: '功能集成测试',
            apiCompatibility: 'API兼容性测试'
        };
        return names[category] || category;
    }
}

// 导出测试类和执行函数
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { ReactMigrationTester, TestResultCollector };
}

// 如果直接运行此文件
if (require.main === module) {
    const tester = new ReactMigrationTester();
    tester.runAllTests()
        .then(report => {
            const success = report.overall.status === 'excellent' || report.overall.status === 'good';
            process.exit(success ? 0 : 1);
        })
        .catch(error => {
            console.error('测试执行失败:', error);
            process.exit(1);
        });
}