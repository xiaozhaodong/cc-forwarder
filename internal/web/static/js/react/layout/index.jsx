// React布局系统入口文件
import App from './App.jsx';
import Header from './components/Header.jsx';
import Navigation from './components/Navigation.jsx';
import MainContent from './components/MainContent.jsx';
import ConnectionIndicator from './components/ConnectionIndicator.jsx';
import ErrorBoundary from './components/ErrorBoundary.jsx';
import { AppStateProvider, useAppState } from './hooks/useAppState.jsx';
import { useNavigation } from './hooks/useNavigation.jsx';

// 导出主应用组件
export default App;

// 导出所有布局组件
export {
    App,
    Header,
    Navigation,
    MainContent,
    ConnectionIndicator,
    ErrorBoundary,
    AppStateProvider,
    useAppState,
    useNavigation
};