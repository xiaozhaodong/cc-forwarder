/**
 * RequestsTable - 请求表格组件
 * 文件描述: 显示请求列表和分页控制的主要表格组件，支持12列数据布局
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';
import TableHeader from './TableHeader.jsx';
import TableBody from './TableBody.jsx';
import PaginationControl from './PaginationControl.jsx';

const RequestsTable = ({
    requests = [],
    isInitialLoading = false,
    onRowClick,
    // usePagination Hook的所有返回值
    pagination,
    // 顶部操作区域相关
    onRefresh
}) => {
    const {
        currentPage,
        totalPages,
        pageSize,
        totalCount,
        rangeText,
        canGoPrev,
        canGoNext,
        PAGE_SIZE_OPTIONS,
        goToPage,
        goToPrevPage,
        goToNextPage,
        goToFirstPage,
        goToLastPage,
        changePageSize
    } = pagination;

    // 计算显示的记录范围文本
    const getRecordsInfo = () => {
        if (totalCount === 0) {
            return "显示 0-0 条，共 0 条记录";
        }

        const start = (currentPage - 1) * pageSize + 1;
        const end = Math.min(currentPage * pageSize, totalCount);
        return `显示 ${start}-${end} 条，共 ${totalCount} 条记录`;
    };

    return (
        <div className="requests-table">
            {/* 表格顶部操作区域 */}
            <div className="table-header">
                <h3>请求详情列表</h3>
                <div className="table-actions">
                    <span className="requests-count-info">
                        <span className="table-tips">
                            <span className="tip-text">单击复制 · 双击查看详情</span>
                            <span className="tip-separator">｜</span>
                        </span>
                        {getRecordsInfo()}
                    </span>
                    <button
                        className="btn btn-sm"
                        onClick={onRefresh}
                        title="刷新数据"
                    >
                        🔄 刷新
                    </button>
                </div>
            </div>

            {/* 表格容器 */}
            <div className="table-container">
                <table className="table">
                    <TableHeader />
                    <TableBody
                        requests={requests}
                        onRowClick={onRowClick}
                        isInitialLoading={isInitialLoading}
                    />
                </table>
            </div>

            {/* 分页控制 - 只在非初次加载状态且有数据时显示 */}
            {!isInitialLoading && totalCount > 0 && (
                <PaginationControl
                    // 状态属性
                    currentPage={currentPage}
                    totalPages={totalPages}
                    pageSize={pageSize}
                    totalCount={totalCount}
                    rangeText={rangeText}
                    canGoPrev={canGoPrev}
                    canGoNext={canGoNext}
                    PAGE_SIZE_OPTIONS={PAGE_SIZE_OPTIONS}

                    // 方法属性
                    goToPage={goToPage}
                    goToPrevPage={goToPrevPage}
                    goToNextPage={goToNextPage}
                    goToFirstPage={goToFirstPage}
                    goToLastPage={goToLastPage}
                    changePageSize={changePageSize}
                />
            )}
        </div>
    );
};

export default RequestsTable;

