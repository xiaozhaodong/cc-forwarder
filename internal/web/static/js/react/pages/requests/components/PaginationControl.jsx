/**
 * PaginationControl - 分页控制组件
 * 文件描述: 提供完整的分页导航和页面大小控制，集成usePagination Hook
 * 创建时间: 2025-09-20 19:23:22
 * 功能特性:
 * - 分页信息显示（X-Y条，共Z条记录）
 * - 每页显示数量选择（20, 50, 100, 200）
 * - 完整分页导航（首页/上一页/下一页/末页）
 * - 当前页码输入框支持
 * - 键盘导航和可访问性支持
 * - 响应式布局适配
 */

import React, { useState, useCallback } from 'react';

const PaginationControl = ({
    // 从usePagination Hook获取的状态
    currentPage,
    totalPages,
    pageSize,
    totalCount,
    rangeText,
    canGoPrev,
    canGoNext,
    PAGE_SIZE_OPTIONS,

    // 从usePagination Hook获取的方法
    goToPage,
    goToPrevPage,
    goToNextPage,
    goToFirstPage,
    goToLastPage,
    changePageSize
}) => {
    // 页码输入框的本地状态
    const [pageInput, setPageInput] = useState('');
    const [showPageInput, setShowPageInput] = useState(false);

    // 处理页码输入框的变化
    const handlePageInputChange = useCallback((e) => {
        const value = e.target.value;
        // 只允许数字输入
        if (/^\d*$/.test(value)) {
            setPageInput(value);
        }
    }, []);

    // 处理页码输入框的确认
    const handlePageInputSubmit = useCallback((e) => {
        if (e.type === 'keydown' && e.key !== 'Enter') return;

        const pageNumber = parseInt(pageInput, 10);
        if (pageNumber && pageNumber >= 1 && pageNumber <= totalPages) {
            goToPage(pageNumber);
        }
        setPageInput('');
        setShowPageInput(false);
    }, [pageInput, totalPages, goToPage]);

    // 处理页面大小变更
    const handlePageSizeChange = useCallback((e) => {
        const newPageSize = parseInt(e.target.value, 10);
        changePageSize(newPageSize);
    }, [changePageSize]);

    // 显示页码输入框
    const handleShowPageInput = useCallback(() => {
        setShowPageInput(true);
        setPageInput(currentPage.toString());
    }, [currentPage]);

    // 隐藏页码输入框
    const handleHidePageInput = useCallback(() => {
        setShowPageInput(false);
        setPageInput('');
    }, []);

    // 如果没有数据，不显示分页控制
    if (totalCount === 0) {
        return null;
    }

    return (
        <div className="pagination-container">
            {/* 分页信息显示 */}
            <div className="pagination-info">
                <span className="pagination-range">{rangeText}</span>
            </div>

            {/* 中央分页导航区域 */}
            <div className="pagination-navigation">
                {/* 首页按钮 */}
                <button
                    className={`pagination-btn pagination-btn-first ${!canGoPrev ? 'disabled' : ''}`}
                    onClick={goToFirstPage}
                    disabled={!canGoPrev}
                    title="首页"
                    aria-label="首页"
                >
                    ««
                </button>

                {/* 上一页按钮 */}
                <button
                    className={`pagination-btn pagination-btn-prev ${!canGoPrev ? 'disabled' : ''}`}
                    onClick={goToPrevPage}
                    disabled={!canGoPrev}
                    title="上一页"
                    aria-label="上一页"
                >
                    ‹
                </button>

                {/* 页码显示/输入区域 */}
                <div className="pagination-page-info">
                    {showPageInput ? (
                        <div className="pagination-page-input-wrapper">
                            <input
                                type="text"
                                className="pagination-page-input"
                                value={pageInput}
                                onChange={handlePageInputChange}
                                onKeyDown={handlePageInputSubmit}
                                onBlur={handleHidePageInput}
                                placeholder={`1-${totalPages}`}
                                maxLength={totalPages.toString().length}
                                autoFocus
                                title="输入页码后按回车确认"
                                aria-label="页码输入框"
                            />
                            <span className="pagination-page-total">/ {totalPages}</span>
                        </div>
                    ) : (
                        <button
                            className="pagination-page-display"
                            onClick={handleShowPageInput}
                            title="点击输入页码"
                            aria-label={`当前第${currentPage}页，共${totalPages}页，点击输入页码`}
                        >
                            {currentPage} / {totalPages}
                        </button>
                    )}
                </div>

                {/* 下一页按钮 */}
                <button
                    className={`pagination-btn pagination-btn-next ${!canGoNext ? 'disabled' : ''}`}
                    onClick={goToNextPage}
                    disabled={!canGoNext}
                    title="下一页"
                    aria-label="下一页"
                >
                    ›
                </button>

                {/* 末页按钮 */}
                <button
                    className={`pagination-btn pagination-btn-last ${!canGoNext ? 'disabled' : ''}`}
                    onClick={goToLastPage}
                    disabled={!canGoNext}
                    title="末页"
                    aria-label="末页"
                >
                    »»
                </button>
            </div>

            {/* 页面大小选择器 */}
            <div className="pagination-page-size">
                <label htmlFor="page-size-select" className="pagination-page-size-label">
                    每页显示：
                </label>
                <select
                    id="page-size-select"
                    className="pagination-page-size-select"
                    value={pageSize}
                    onChange={handlePageSizeChange}
                    title="选择每页显示的记录数"
                    aria-label="每页显示记录数选择"
                >
                    {PAGE_SIZE_OPTIONS.map(size => (
                        <option key={size} value={size}>
                            {size} 条
                        </option>
                    ))}
                </select>
            </div>
        </div>
    );
};

export default PaginationControl;