// React错误边界组件
import React from 'react';

class ErrorBoundary extends React.Component {
    constructor(props) {
        super(props);
        this.state = { hasError: false, error: null, errorInfo: null };
    }

    static getDerivedStateFromError(error) {
        // 更新 state 使下一次渲染能够显示降级后的 UI
        return { hasError: true };
    }

    componentDidCatch(error, errorInfo) {
        // 你同样可以将错误日志上报给服务器
        console.error('🚨 [错误边界] React组件错误:', error, errorInfo);
        this.setState({
            error: error,
            errorInfo: errorInfo
        });
    }

    render() {
        if (this.state.hasError) {
            // 你可以自定义降级后的 UI 并渲染
            return (
                <div style={{
                    textAlign: 'center',
                    padding: '48px 24px',
                    color: '#ef4444',
                    border: '1px solid #fecaca',
                    borderRadius: '8px',
                    background: '#fef2f2',
                    margin: '20px'
                }}>
                    <div style={{ fontSize: '48px', marginBottom: '16px' }}>🚨</div>
                    <h3 style={{ margin: '0 0 16px 0', color: '#dc2626' }}>组件运行时错误</h3>
                    <p style={{ margin: '0 0 16px 0', fontSize: '14px', color: '#7f1d1d' }}>
                        {this.state.error && this.state.error.toString()}
                    </p>
                    <details style={{ textAlign: 'left', margin: '16px 0', fontSize: '12px', color: '#7f1d1d' }}>
                        <summary style={{ cursor: 'pointer', marginBottom: '8px' }}>查看详细错误信息</summary>
                        <pre style={{
                            background: '#f3f4f6',
                            padding: '8px',
                            borderRadius: '4px',
                            overflow: 'auto',
                            maxHeight: '200px'
                        }}>
                            {this.state.errorInfo?.componentStack || '错误堆栈信息不可用'}
                        </pre>
                    </details>
                    <button
                        onClick={() => window.location.reload()}
                        style={{
                            padding: '8px 16px',
                            background: '#dc2626',
                            color: 'white',
                            border: 'none',
                            borderRadius: '6px',
                            cursor: 'pointer',
                            marginRight: '8px'
                        }}
                    >
                        重新加载页面
                    </button>
                    <button
                        onClick={() => this.setState({ hasError: false, error: null, errorInfo: null })}
                        style={{
                            padding: '8px 16px',
                            background: '#6b7280',
                            color: 'white',
                            border: 'none',
                            borderRadius: '6px',
                            cursor: 'pointer'
                        }}
                    >
                        重试组件
                    </button>
                </div>
            );
        }

        return this.props.children;
    }
}

export default ErrorBoundary;