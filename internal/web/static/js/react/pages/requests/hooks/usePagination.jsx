/**
 * usePagination - 分页状态管理Hook
 * 文件描述: 管理表格数据的分页逻辑、状态和导航功能
 * 创建时间: 2025-09-20 18:03:21
 * 功能: 分页状态管理、页码计算、导航逻辑
 * 支持页面大小: 20, 50, 100, 200
 * 默认页面大小: 50
 */

import React, { useState, useCallback, useMemo, useEffect } from 'react';

// 分页配置常量
const PAGE_SIZE_OPTIONS = [20, 50, 100, 200];
const DEFAULT_PAGE_SIZE = 50;

export const usePagination = (totalCount = 0, initialPageSize = DEFAULT_PAGE_SIZE) => {
    const [currentPage, setCurrentPage] = useState(1);
    const [pageSize, setPageSize] = useState(() => {
        // 验证初始页面大小是否在允许的选项中
        return PAGE_SIZE_OPTIONS.includes(initialPageSize) ? initialPageSize : DEFAULT_PAGE_SIZE;
    });

    // 计算总页数
    const totalPages = useMemo(() => {
        return Math.max(1, Math.ceil(totalCount / pageSize));
    }, [totalCount, pageSize]);

    // 计算当前页的起始索引（从0开始）
    const startIndex = useMemo(() => {
        return (currentPage - 1) * pageSize;
    }, [currentPage, pageSize]);

    // 计算当前页的结束索引
    const endIndex = useMemo(() => {
        return Math.min(startIndex + pageSize, totalCount);
    }, [startIndex, pageSize, totalCount]);

    // 计算当前页显示的记录范围文本
    const rangeText = useMemo(() => {
        if (totalCount === 0) {
            return '暂无数据';
        }
        const start = startIndex + 1;
        const end = endIndex;
        return `显示 ${start}-${end} 条，共 ${totalCount} 条记录`;
    }, [startIndex, endIndex, totalCount]);

    // 检查是否可以跳转到上一页
    const canGoPrev = useMemo(() => {
        return currentPage > 1;
    }, [currentPage]);

    // 检查是否可以跳转到下一页
    const canGoNext = useMemo(() => {
        return currentPage < totalPages;
    }, [currentPage, totalPages]);

    // 跳转到指定页码
    const goToPage = useCallback((page) => {
        const targetPage = Math.max(1, Math.min(page, totalPages));
        if (targetPage !== currentPage) {
            setCurrentPage(targetPage);
        }
    }, [currentPage, totalPages]);

    // 跳转到上一页
    const goToPrevPage = useCallback(() => {
        if (canGoPrev) {
            setCurrentPage(prev => prev - 1);
        }
    }, [canGoPrev]);

    // 跳转到下一页
    const goToNextPage = useCallback(() => {
        if (canGoNext) {
            setCurrentPage(prev => prev + 1);
        }
    }, [canGoNext]);

    // 跳转到第一页
    const goToFirstPage = useCallback(() => {
        if (currentPage !== 1) {
            setCurrentPage(1);
        }
    }, [currentPage]);

    // 跳转到最后一页
    const goToLastPage = useCallback(() => {
        if (currentPage !== totalPages) {
            setCurrentPage(totalPages);
        }
    }, [currentPage, totalPages]);

    // 更改页面大小
    const changePageSize = useCallback((newPageSize) => {
        if (!PAGE_SIZE_OPTIONS.includes(newPageSize)) {
            console.warn(`Invalid page size: ${newPageSize}. Must be one of: ${PAGE_SIZE_OPTIONS.join(', ')}`);
            return;
        }

        // 计算新的总页数
        const newTotalPages = Math.max(1, Math.ceil(totalCount / newPageSize));

        // 尝试保持用户当前查看的数据位置
        const currentFirstItemIndex = (currentPage - 1) * pageSize;
        const newCurrentPage = Math.max(1, Math.min(
            Math.floor(currentFirstItemIndex / newPageSize) + 1,
            newTotalPages
        ));

        setPageSize(newPageSize);
        setCurrentPage(newCurrentPage);
    }, [currentPage, pageSize, totalCount]);

    // 重置分页状态
    const resetPagination = useCallback((newPageSize = DEFAULT_PAGE_SIZE) => {
        setCurrentPage(1);
        if (PAGE_SIZE_OPTIONS.includes(newPageSize)) {
            setPageSize(newPageSize);
        }
    }, []);

    // 获取分页API参数
    const getPaginationParams = useCallback(() => {
        return {
            limit: pageSize,
            offset: startIndex,
            page: currentPage
        };
    }, [pageSize, startIndex, currentPage]);

    // 当总数变化时，确保当前页不超出范围
    useEffect(() => {
        if (totalCount > 0 && currentPage > totalPages) {
            setCurrentPage(totalPages);
        }
    }, [totalCount, currentPage, totalPages]);

    // 生成页码数组用于分页导航渲染
    const getPageNumbers = useCallback((maxVisible = 7) => {
        const pages = [];
        const half = Math.floor(maxVisible / 2);

        let start = Math.max(1, currentPage - half);
        let end = Math.min(totalPages, currentPage + half);

        // 调整范围以显示足够的页码
        if (end - start + 1 < maxVisible) {
            if (start === 1) {
                end = Math.min(totalPages, start + maxVisible - 1);
            } else if (end === totalPages) {
                start = Math.max(1, end - maxVisible + 1);
            }
        }

        for (let i = start; i <= end; i++) {
            pages.push(i);
        }

        return pages;
    }, [currentPage, totalPages]);

    return {
        // 状态信息
        currentPage,        // 当前页码
        totalPages,         // 总页数
        pageSize,           // 每页大小
        totalCount,         // 总记录数
        startIndex,         // 起始索引
        endIndex,           // 结束索引
        rangeText,          // 范围文本

        // 导航能力
        canGoPrev,          // 是否可以上一页
        canGoNext,          // 是否可以下一页

        // 导航方法
        goToPage,           // 跳转到指定页
        goToPrevPage,       // 上一页
        goToNextPage,       // 下一页
        goToFirstPage,      // 第一页
        goToLastPage,       // 最后一页
        changePageSize,     // 改变页面大小
        resetPagination,    // 重置分页

        // 工具方法
        getPaginationParams,// 获取API分页参数
        getPageNumbers,     // 获取页码数组

        // 常量
        PAGE_SIZE_OPTIONS,  // 页面大小选项
        DEFAULT_PAGE_SIZE   // 默认页面大小
    };
};

// 默认导出主要的Hook（支持两种导入方式）
export default usePagination;