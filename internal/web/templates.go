package web

// indexHTML contains the React-based HTML template for the web interface
const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Claude Request Forwarder - Web界面</title>
    <link rel="stylesheet" href="/static/css/style.css">
    <link rel="stylesheet" href="/static/css/layout.css">
    <link rel="stylesheet" href="/static/css/requests-react.css">

    <!-- Chart.js with fallback and timeout -->
    <script>
    window.chartLoadTimeout = setTimeout(() => {
        if (!window.Chart) {
            console.warn('Chart.js CDN loading timeout, charts will be disabled');
            window.chartLoadFailed = true;
        }
    }, 3000);
    </script>
    <script src="/static/js/lib/chart.umd.js"
            onload="clearTimeout(window.chartLoadTimeout); console.log('Chart.js loaded successfully');"
            onerror="window.chartLoadFailed=true; clearTimeout(window.chartLoadTimeout); console.warn('Chart.js local file failed, charts disabled');"></script>

    <!-- React Libraries (本地化依赖) -->
    <script>
    // React loading timeout handling
    window.reactLoadTimeout = setTimeout(() => {
        console.warn('React loading timeout, React components will be disabled');
        window.reactLoadFailed = true;
    }, 5000);
    </script>
    <script src="/static/js/lib/react.development.js"
            onload="console.log('React loaded successfully');"
            onerror="window.reactLoadFailed=true; clearTimeout(window.reactLoadTimeout); console.warn('React failed to load, fallback to legacy UI');"></script>
    <script src="/static/js/lib/react-dom.development.js"
            onload="console.log('ReactDOM loaded successfully');"
            onerror="window.reactLoadFailed=true; clearTimeout(window.reactLoadTimeout); console.warn('ReactDOM failed to load, fallback to legacy UI');"></script>
    <script src="/static/js/lib/babel.min.js"
            onload="clearTimeout(window.reactLoadTimeout); console.log('Babel loaded successfully, React system ready');"
            onerror="window.reactLoadFailed=true; clearTimeout(window.reactLoadTimeout); console.warn('Babel failed to load, JSX disabled');"></script>
</head>
<body>
    <!-- React应用挂载点 -->
    <div id="react-app-root">
        <div style="
            min-height: 100vh;
            background-color: #f8fafc;
            padding: 20px;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif;
        ">
            <!-- 模拟header结构 -->
            <div style="
                max-width: 1400px;
                margin: 0 auto;
                padding: 20px;
                background: #ffffff;
                border-radius: 12px;
                box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
                text-align: center;
                margin-bottom: 30px;
            ">
                <h1 style="
                    color: #2563eb;
                    font-size: 2.5em;
                    margin: 0 0 8px 0;
                    font-weight: 600;
                ">🌐 Claude Request Forwarder</h1>
                <p style="
                    color: #64748b;
                    font-size: 1.1em;
                    margin: 0;
                ">高性能API请求转发器 - Web监控界面</p>
            </div>

            <!-- 加载内容区域 -->
            <div style="
                max-width: 1400px;
                margin: 0 auto;
                padding: 60px 20px;
                background: #ffffff;
                border-radius: 12px;
                box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
                text-align: center;
            ">
                <!-- 加载动画 -->
                <div style="
                    width: 50px;
                    height: 50px;
                    border: 3px solid #e2e8f0;
                    border-top: 3px solid #2563eb;
                    border-radius: 50%;
                    animation: spin 1s linear infinite;
                    margin: 0 auto 24px auto;
                "></div>

                <!-- 加载文字 -->
                <h3 style="
                    color: #1e293b;
                    font-size: 18px;
                    font-weight: 600;
                    margin: 0 0 8px 0;
                ">系统初始化中</h3>

                <p style="
                    color: #64748b;
                    font-size: 14px;
                    margin: 0 0 24px 0;
                ">正在加载管理界面，请稍候...</p>

                <!-- 进度指示器 -->
                <div style="
                    width: 200px;
                    height: 4px;
                    background: #e2e8f0;
                    border-radius: 2px;
                    margin: 0 auto;
                    overflow: hidden;
                ">
                    <div style="
                        width: 40%;
                        height: 100%;
                        background: linear-gradient(90deg, #2563eb, #3b82f6);
                        border-radius: 2px;
                        animation: loading 2s ease-in-out infinite;
                    "></div>
                </div>
            </div>
        </div>

        <!-- CSS 动画定义 -->
        <style>
            @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
            }

            @keyframes loading {
                0% {
                    transform: translateX(-100%);
                }
                50% {
                    transform: translateX(250%);
                }
                100% {
                    transform: translateX(-100%);
                }
            }
        </style>
    </div>

    <!-- React模块化系统 -->
    <script src="/static/js/react/registry.js"></script>
    <script src="/static/js/react/moduleLoader.js"></script>

    <script>
        // 初始化React应用
        let reactApp = null;

        // 等待页面完全加载后再启动React应用
        window.addEventListener('load', function() {
            console.log('📱 [React App] 开始启动React布局应用...');

            // 监听React系统就绪事件
            document.addEventListener('reactSystemReady', async function(event) {
                console.log('✅ [React App] React系统就绪，启动主应用');
                await initializeReactApp();
            });

            // 如果React系统已经就绪，直接启动
            if (window.ReactComponents?.isReactReady()) {
                setTimeout(async () => {
                    await initializeReactApp();
                }, 500);
            }
        });

        // 初始化React应用
        async function initializeReactApp() {
            const container = document.getElementById('react-app-root');
            if (!container) {
                console.error('❌ [React App] 找不到React应用挂载点');
                return;
            }

            try {
                console.log('📦 [React App] 开始加载React布局应用...');

                // 使用模块加载器动态导入布局应用
                const LayoutAppModule = await window.importReactModule('layout/index.jsx');
                const LayoutApp = LayoutAppModule.default || LayoutAppModule;

                if (!LayoutApp) {
                    throw new Error('React布局应用模块加载失败');
                }

                console.log('✅ [React App] React布局应用模块加载成功');

                // 创建并渲染React应用
                const appComponent = React.createElement(LayoutApp);
                const success = window.ReactComponents.renderComponent(appComponent, container);

                if (success) {
                    reactApp = appComponent;
                    console.log('✅ [React App] React布局应用启动成功');
                } else {
                    throw new Error('React应用渲染失败');
                }

            } catch (error) {
                console.error('❌ [React App] React应用启动失败:', error);

                // 显示错误信息
                container.innerHTML =
                    '<div style="' +
                        'min-height: 100vh;' +
                        'background-color: #f8fafc;' +
                        'padding: 20px;' +
                        'font-family: -apple-system, BlinkMacSystemFont, \"Segoe UI\", \"PingFang SC\", \"Hiragino Sans GB\", \"Microsoft YaHei\", \"Helvetica Neue\", Helvetica, Arial, sans-serif;' +
                    '">' +
                        '<div style="' +
                            'max-width: 1400px;' +
                            'margin: 0 auto;' +
                            'padding: 20px;' +
                            'background: #ffffff;' +
                            'border-radius: 12px;' +
                            'box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);' +
                            'text-align: center;' +
                            'margin-bottom: 30px;' +
                        '">' +
                            '<h1 style="' +
                                'color: #2563eb;' +
                                'font-size: 2.5em;' +
                                'margin: 0 0 8px 0;' +
                                'font-weight: 600;' +
                            '">🌐 Claude Request Forwarder</h1>' +
                            '<p style="' +
                                'color: #64748b;' +
                                'font-size: 1.1em;' +
                                'margin: 0;' +
                            '">高性能API请求转发器 - Web监控界面</p>' +
                        '</div>' +
                        '<div style="' +
                            'max-width: 1400px;' +
                            'margin: 0 auto;' +
                            'padding: 60px 20px;' +
                            'background: #ffffff;' +
                            'border-radius: 12px;' +
                            'box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);' +
                            'text-align: center;' +
                        '">' +
                            '<div style="' +
                                'width: 64px;' +
                                'height: 64px;' +
                                'background: #fef2f2;' +
                                'border-radius: 50%;' +
                                'display: flex;' +
                                'align-items: center;' +
                                'justify-content: center;' +
                                'margin: 0 auto 24px auto;' +
                            '">' +
                                '<div style="color: #dc2626; font-size: 32px;">⚠️</div>' +
                            '</div>' +
                            '<h3 style="' +
                                'color: #1e293b;' +
                                'font-size: 20px;' +
                                'font-weight: 600;' +
                                'margin: 0 0 8px 0;' +
                            '">系统初始化失败</h3>' +
                            '<p style="' +
                                'color: #64748b;' +
                                'font-size: 14px;' +
                                'margin: 0 0 32px 0;' +
                                'line-height: 1.5;' +
                            '">管理界面加载遇到问题，请检查网络连接后重试</p>' +
                            '<button onclick="window.location.reload()" ' +
                                   'style="' +
                                       'padding: 12px 24px;' +
                                       'background: #2563eb;' +
                                       'color: white;' +
                                       'border: none;' +
                                       'border-radius: 8px;' +
                                       'cursor: pointer;' +
                                       'font-size: 14px;' +
                                       'font-weight: 500;' +
                                       'transition: background-color 0.2s ease;' +
                                   '" ' +
                                   'onmouseover="this.style.background=\'#1d4ed8\'" ' +
                                   'onmouseout="this.style.background=\'#2563eb\'">' +
                                '重新加载' +
                            '</button>' +
                        '</div>' +
                    '</div>';
            }
        }

        // 页面卸载时清理React应用
        window.addEventListener('beforeunload', () => {
            if (reactApp) {
                const container = document.getElementById('react-app-root');
                if (container) {
                    window.ReactComponents.unmountComponent(container);
                }
            }
        });
    </script>
</body>
</html>`